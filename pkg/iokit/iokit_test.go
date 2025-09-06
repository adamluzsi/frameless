package iokit_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"

	"go.llib.dev/testcase"
)

var rnd = random.New(random.CryptoSeed{})

type RWSC interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
}

var _ RWSC = &iokit.Buffer{}

func TestBuffer(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		data = testcase.Var[[]byte]{ID: "seek offset", Init: func(t *testcase.T) []byte {
			return []byte(t.Random.String())
		}}
		buffer = testcase.Var[*iokit.Buffer]{ID: "*iokit.Buffer", Init: func(t *testcase.T) *iokit.Buffer {
			return iokit.NewBuffer(data.Get(t))
		}}
		rwsc = testcase.Var[RWSC]{ID: "reference reader/writer/seeker/closer", Init: func(t *testcase.T) RWSC {
			name := t.Random.StringNWithCharset(5, "qwerty")
			path := filepath.Join(t.TempDir(), name)
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filemode.UserRWX)
			t.Must.NoError(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data.Get(t))
			t.Must.NoError(err)
			t.Must.Equal(len(data.Get(t)), n)
			t.Must.NoError(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			t.Must.NoError(err)
			t.Defer(f.Close)
			return f
		}}
	)

	thenContentsAreMatching := func(s *testcase.Spec) {
		s.Then("contents are matching with the reference ReadWriteSeeker", func(t *testcase.T) {
			expected, err := io.ReadAll(rwsc.Get(t))
			t.Must.NoError(err)
			actual, err := io.ReadAll(buffer.Get(t))
			t.Must.NoError(err)
			t.Must.Equal(string(expected), string(actual))
		})
	}

	thenContentsAreMatching(s)

	seekOnBoth := func(t *testcase.T, offset int64, whench int) {
		expected, err := rwsc.Get(t).Seek(offset, whench)
		t.Must.NoError(err)
		actual, err := buffer.Get(t).Seek(offset, whench)
		t.Must.NoError(err)
		t.Must.Equal(expected, actual, "seek length matches")
	}

	writeOnBoth := func(t *testcase.T, bs []byte) {
		expected, err := rwsc.Get(t).Write(bs)
		t.Must.NoError(err)
		actual, err := buffer.Get(t).Write(bs)
		t.Must.NoError(err)
		t.Must.Equal(expected, actual, "write length matches")
	}

	s.When("when seek is made", func(s *testcase.Spec) {
		offset := testcase.Let(s, func(t *testcase.T) int64 {
			// max offset is around half of the total data size
			return int64(t.Random.IntN(1 + len(data.Get(t))/2))
		})
		whench := testcase.Let[int](s, nil)

		thenAfterSeekContentsAreMatching := func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seekOnBoth(t, offset.Get(t), whench.Get(t))
			})

			thenContentsAreMatching(s)

			s.And("write is made after seeking", func(s *testcase.Spec) {
				wData := testcase.Let(s, func(t *testcase.T) []byte {
					return []byte(t.Random.StringN(len(string(data.Get(t)))))
				})
				s.Before(func(t *testcase.T) {
					writeOnBoth(t, wData.Get(t))
				})

				thenContentsAreMatching(s)
			})
		}

		s.And("from the start", func(s *testcase.Spec) {
			whench.LetValue(s, io.SeekStart)

			thenAfterSeekContentsAreMatching(s)
		})

		s.And("from the current that is somewhere middle", func(s *testcase.Spec) {
			whench.LetValue(s, io.SeekCurrent)
			s.Before(func(t *testcase.T) {
				seekOnBoth(t, int64(t.Random.IntN(1+len(data.Get(t)))), io.SeekStart)
			})

			thenAfterSeekContentsAreMatching(s)
		})

		s.And("from the end", func(s *testcase.Spec) {
			whench.LetValue(s, io.SeekEnd)

			thenAfterSeekContentsAreMatching(s)
		})

		s.And("the seeking offset would point to an", func(s *testcase.Spec) {
			offset.LetValue(s, -42)
			whench.LetValue(s, io.SeekStart)

			s.Then(".Seek will fail because the negative position", func(t *testcase.T) {
				_, err := rwsc.Get(t).Seek(offset.Get(t), whench.Get(t))
				var pathError *fs.PathError
				t.Must.True(errors.As(err, &pathError))
				_, err = buffer.Get(t).Seek(offset.Get(t), whench.Get(t))
				t.Must.ErrorIs(iokit.ErrSeekNegativePosition, err)
			})
		})
	})

	s.When("data is additionally written", func(s *testcase.Spec) {
		s.And("write is made after seeking", func(s *testcase.Spec) {
			wData := testcase.Let(s, func(t *testcase.T) []byte {
				return []byte(t.Random.StringN(len(string(data.Get(t)))))
			})
			s.Before(func(t *testcase.T) {
				writeOnBoth(t, wData.Get(t))
			})

			thenContentsAreMatching(s)
		})
	})

	s.When(".Close is called", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Must.NoError(buffer.Get(t).Close())
			t.Must.NoError(rwsc.Get(t).Close())
		})

		s.Then("simultaneous .Close yields error", func(t *testcase.T) {
			err := rwsc.Get(t).Close()
			t.Must.ErrorIs(fs.ErrClosed, err)
			err = buffer.Get(t).Close()
			t.Must.ErrorIs(fs.ErrClosed, err)
		})

		s.Then(".Read fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Read([]byte{})
			t.Must.ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Read([]byte{})
			t.Must.ErrorIs(fs.ErrClosed, err)
		})

		s.Then(".Write fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Write([]byte{})
			t.Must.ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Write([]byte{})
			t.Must.ErrorIs(fs.ErrClosed, err)
		})

		s.Then(".Seek fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Seek(0, io.SeekStart)
			t.Must.ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Seek(0, io.SeekStart)
			t.Must.ErrorIs(fs.ErrClosed, err)
		})
	})

	s.Describe(".String and .Bytes", func(s *testcase.Spec) {
		thenStringAndBytesResultsAreMatchesTheRefRWSCReadContent := func(s *testcase.Spec) {
			thenContentsAreMatching(s)

			s.Then(".String() matches with reference rwsc's read content", func(t *testcase.T) {
				rwsc.Get(t).Seek(0, io.SeekStart)
				expectedBS, err := io.ReadAll(rwsc.Get(t))
				t.Must.NoError(err)
				t.Must.Equal(string(expectedBS), buffer.Get(t).String())
				t.Must.Equal(expectedBS, buffer.Get(t).Bytes())
			})

			s.Then(".Bytes() matches with reference rwsc's read content", func(t *testcase.T) {
				rwsc.Get(t).Seek(0, io.SeekStart)
				expectedBS, err := io.ReadAll(rwsc.Get(t))
				t.Must.NoError(err)
				t.Must.Equal(string(expectedBS), buffer.Get(t).String())
				t.Must.Equal(expectedBS, buffer.Get(t).Bytes())
			})
		}

		thenStringAndBytesResultsAreMatchesTheRefRWSCReadContent(s)

		s.And("seeking is made", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seekOnBoth(t, int64(t.Random.IntN(1+len(data.Get(t))/2)), io.SeekStart)
			})

			thenStringAndBytesResultsAreMatchesTheRefRWSCReadContent(s)
		})

		s.Context("write is made", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				writeOnBoth(t, []byte(t.Random.String()))
			})

			thenStringAndBytesResultsAreMatchesTheRefRWSCReadContent(s)
		})
	})
}

func TestBuffer_ioReadWriter(t *testing.T) {
	var (
		msg = []byte("Hello, world!\n")
		buf = &iokit.Buffer{}
	)

	n, err := buf.Write(append([]byte{}, msg...))
	assert.NoError(t, err)
	assert.Equal(t, n, len(msg))
	assert.Equal(t, msg, buf.Bytes())

	_, err = buf.Seek(0, io.SeekStart)
	assert.NoError(t, err)

	bs := make([]byte, len(buf.Bytes()))
	n, err = buf.Read(bs)
	assert.NoError(t, err)
	assert.Equal(t, n, len(bs))
	assert.Equal(t, msg, bs)
}

func TestBuffer_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	var (
		data   = []byte(t.Random.String())
		offset = int64(t.Random.IntN(1 + len(data)))
		whench = t.Random.Pick([]int{io.SeekStart, io.SeekCurrent, io.SeekEnd}).(int)
		buffer = iokit.NewBuffer(data)
		tmpDir = t.TempDir()
		file   = func(t *testcase.T) filesystem.File {
			name := t.Random.StringNWithCharset(5, "qwerty")
			path := filepath.Join(tmpDir, name)
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filemode.UserRWX)
			t.Must.NoError(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data)
			t.Must.NoError(err)
			t.Must.Equal(len(data), n)
			t.Must.NoError(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			t.Must.NoError(err)
			t.Defer(f.Close)
			return f
		}(t)
	)

	bseek, err := buffer.Seek(offset, whench)
	t.Must.NoError(err)

	fseek, err := file.Seek(offset, whench)
	t.Must.NoError(err)

	t.Must.Equal(fseek, bseek)

	wData := []byte(t.Random.StringN(1))
	bwrite, err := buffer.Write(wData)
	t.Must.NoError(err)
	fwrite, err := file.Write(wData)
	t.Must.NoError(err)
	t.Must.Equal(fwrite, bwrite)

	_, err = file.Seek(0, io.SeekStart)
	t.Must.NoError(err)
	_, err = buffer.Seek(0, io.SeekStart)
	t.Must.NoError(err)

	expectedBS, err := io.ReadAll(file)
	t.Must.NoError(err)
	actualBS, err := io.ReadAll(buffer)
	t.Must.NoError(err)
	t.Must.Equal(string(expectedBS), string(actualBS))
}

func TestNew_smoke(tt *testing.T) {
	t := testcase.NewT(tt)
	dataSTR := t.Random.String()
	dataBS := []byte(t.Random.String())
	t.Must.Equal(iokit.NewBuffer(dataSTR).String(), dataSTR)
	t.Must.Equal(iokit.NewBuffer(dataBS).Bytes(), dataBS)
}

func TestBuffer_Read_ioReadAll(t *testing.T) {
	t.Run("on empty buffer", func(tt *testing.T) {
		t := testcase.NewT(tt)
		b := &iokit.Buffer{}
		bs, err := io.ReadAll(b)
		t.Must.NoError(err)
		t.Must.Empty(bs)
	})
	t.Run("on populated buffer", func(tt *testing.T) {
		t := testcase.NewT(tt)
		d := t.Random.String()
		b := iokit.NewBuffer(d)
		bs, err := io.ReadAll(b)
		t.Must.NoError(err)
		t.Must.Equal(d, string(bs))
	})
}

func TestSyncWriter_smoke(t *testing.T) {
	const msg = "Hello, world!\n"
	buf := &iokit.Buffer{}
	sw := iokit.SyncWriter{Writer: buf}
	do := func() {
		bs := []byte(msg)
		n, err := sw.Write(bs)
		assert.Should(t).NoError(err)
		assert.Should(t).Equal(n, len(bs))
	}
	testcase.Race(do, do, do)
	assert.Equal(t, strings.Repeat(msg, 3), buf.String())
}

func BenchmarkSyncWriter_Write(b *testing.B) {
	const msg = "Hello, world!\n"
	bs := []byte(msg)
	sw := iokit.SyncWriter{Writer: &bytes.Buffer{}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Write(bs)
	}
}

func TestSyncReader_smoke(t *testing.T) {
	const msg = "Hello, world!\n"
	var (
		buf = &bytes.Buffer{}
		m   = &sync.Mutex{}
		w   = iokit.SyncWriter{Writer: buf, Locker: m}
		r   = iokit.SyncReader{Reader: buf, Locker: m}
	)
	do := func() {
		bs := []byte(msg)
		_, _ = w.Write(bs)
		bs = make([]byte, len(bs))
		n, err := r.Read(bs)
		assert.Should(t).NoError(err)
		assert.Should(t).Equal(n, len(bs))
		assert.Should(t).Equal(msg, string(bs))
	}
	testcase.Race(do, do, do)
}

func TestSyncReaderWriter_smoke(t *testing.T) {
	const msg = "Hello, world!\n"
	var (
		buf    = &bytes.Buffer{}
		rw     = iokit.SyncReadWriter{ReadWriter: buf}
		msgLen = len([]byte(msg))
	)

	bs := []byte(msg)
	n, err := rw.Write(bs)
	assert.NoError(t, err)
	assert.Equal(t, n, len(bs))
	assert.Equal(t, msg, buf.String())

	bs = make([]byte, len(buf.Bytes()))
	n, err = rw.Read(bs)
	assert.NoError(t, err)
	assert.Equal(t, n, len(bs))
	assert.Equal(t, msg, string(bs))

	write := func() {
		bs := []byte(msg)
		n, err := rw.Write(bs)
		assert.Should(t).NoError(err)
		assert.Should(t).Equal(n, len(bs))
	}
	read := func() {
		_, err := rw.Read(make([]byte, msgLen))
		assert.Should(t).AnyOf(func(a *assert.A) {
			a.Test(func(t testing.TB) { assert.ErrorIs(t, io.EOF, err) })
			a.Test(func(t testing.TB) { assert.NoError(t, err) })
		})
	}
	testcase.Race(
		write, write, write,
		read, read, read)
}

func TestReadAllWithLimit(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		body := strings.NewReader("foo")
		data, err := iokit.ReadAllWithLimit(body, 5*iokit.Byte)
		assert.NoError(t, err)
		assert.Equal(t, "foo", string(data))
	})
	t.Run("limit reached", func(t *testing.T) {
		body := strings.NewReader("foo")
		data, err := iokit.ReadAllWithLimit(body, 2*iokit.Byte)
		assert.ErrorIs(t, err, iokit.ErrReadLimitReached)
		assert.Empty(t, data)
	})
	t.Run("nil body considered as empty body", func(t *testing.T) {
		data, err := iokit.ReadAllWithLimit(nil, 2*iokit.Byte)
		assert.NoError(t, err)
		assert.Empty(t, data)
	})
	t.Run("error with reader", func(t *testing.T) {
		expErr := rnd.Error()
		body := &iokit.StubReader{
			Data:    []byte("foo"),
			ReadErr: expErr,
		}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.ErrorIs(t, err, expErr)
		assert.Empty(t, data)
	})
	t.Run("Closer type is closed", func(t *testing.T) {
		body := &iokit.StubReader{Data: []byte("foo")}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.NoError(t, err)
		assert.Equal(t, body.Data, data)
		assert.True(t, body.IsClosed())
	})
	t.Run("Closing error is propagated", func(t *testing.T) {
		expErr := rnd.Error()
		body := &iokit.StubReader{
			Data:     []byte("foo"),
			CloseErr: expErr,
		}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.ErrorIs(t, err, expErr)
		assert.Equal(t, data, body.Data)
	})
}

func TestStubReader(t *testing.T) {
	t.Run("behaves like bytes.Reader", func(t *testing.T) {
		data := []byte(rnd.String())

		stubReader := &iokit.StubReader{Data: data}
		referenceReader := bytes.NewReader(data)

		t.Log("on reading the content")
		buf1 := make([]byte, len(data)+1)
		buf2 := make([]byte, len(data)+1)
		expectedN, expectedErr := referenceReader.Read(buf1)
		gotN, gotErr := stubReader.Read(buf2)
		assert.Equal(t, expectedErr, gotErr)
		assert.Equal(t, expectedN, gotN)
		assert.Equal(t, buf1, buf2)

		t.Log("on reading an already exhausted reader")
		buf1 = make([]byte, len(data)+1)
		buf2 = make([]byte, len(data)+1)
		expectedN, expectedErr = referenceReader.Read(buf1)
		gotN, gotErr = stubReader.Read(buf2)
		assert.Equal(t, expectedErr, gotErr)
		assert.Equal(t, expectedN, gotN)
		assert.Equal(t, buf1, buf2)
	})

	t.Run("reads data exact size", func(t *testing.T) {
		data := []byte(rnd.String())
		reader := &iokit.StubReader{Data: data}
		buf := make([]byte, len(data))
		n, err := reader.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, buf, data)
	})

	t.Run("allows the injection of a Read error", func(t *testing.T) {
		readErr := rnd.Error()
		reader := &iokit.StubReader{ReadErr: readErr}

		buf := make([]byte, 1)
		_, err := reader.Read(buf)
		assert.ErrorIs(t, err, readErr)
	})

	t.Run("returns EOF on read past end of data", func(t *testing.T) {
		var total int
		reader := &iokit.StubReader{Data: []byte(rnd.String())}

		buf := make([]byte, len(reader.Data)*2)
		n, err := reader.Read(buf)
		assert.NoError(t, err)
		total += n

		buf = make([]byte, len(reader.Data)*2)
		n, err = reader.Read(buf)
		assert.ErrorIs(t, err, io.EOF)
		total += n

		assert.Equal(t, len(reader.Data), total)
	})

	t.Run("closes without an error", func(t *testing.T) {
		reader := &iokit.StubReader{CloseErr: nil}
		assert.NoError(t, reader.Close())
		assert.True(t, reader.IsClosed())
	})

	t.Run("safe to close multiple times", func(t *testing.T) {
		reader := &iokit.StubReader{CloseErr: nil}
		rnd.Repeat(2, 5, func() {
			assert.NoError(t, reader.Close())
		})
	})

	t.Run("allows the injection of a Close error", func(t *testing.T) {
		expErr := errors.New("test error")
		reader := &iokit.StubReader{CloseErr: expErr}
		err := reader.Close()
		assert.ErrorIs(t, expErr, err)
	})

	t.Run("we can determine when the reading will update the LastReadAt's result", func(t *testing.T) {
		now := clock.Now()
		timecop.Travel(t, now, timecop.Freeze)

		data := []byte(rnd.StringN(5))
		stub := &iokit.StubReader{Data: data}
		assert.Empty(t, stub.LastReadAt())

		n, err := stub.Read(make([]byte, 1))
		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, stub.LastReadAt(), now)

		now = now.Add(time.Hour)
		timecop.Travel(t, now, timecop.Freeze)
		assert.NotEqual(t, stub.LastReadAt(), now)

		n, err = stub.Read(make([]byte, 1))
		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Equal(t, stub.LastReadAt(), now)
	})

	t.Run("LastReadAt is thread safe", func(t *testing.T) {
		stub := &iokit.StubReader{Data: []byte(rnd.StringN(5))}

		testcase.Race(func() {
			stub.LastReadAt()
		}, func() {
			stub.LastReadAt()
		}, func() {
			_, _ = stub.Read(make([]byte, 1))
		})
	})

	t.Run("IsClosed is thread safe", func(t *testing.T) {
		stub := &iokit.StubReader{Data: []byte(rnd.StringN(5))}

		testcase.Race(func() {
			_ = stub.IsClosed()
		}, func() {
			_ = stub.Close()
		})
	})

	t.Run("after closing the reader, LastReadAt still records reading attempts", func(t *testing.T) {
		stub := &iokit.StubReader{Data: []byte(rnd.StringN(5))}
		assert.NoError(t, stub.Close())
		_, _ = stub.Read(make([]byte, 1))
		assert.NotEmpty(t, stub.LastReadAt())
	})
}

func ExampleKeepAliveReader() {
	r := bytes.NewReader([]byte("reader"))

	kar := iokit.NewKeepAliveReader(r, 20*time.Second)
	defer kar.Close()
	var _ io.ReadCloser = kar

	_, _ = kar.Read(make([]byte, 10))
}

func TestKeepAliveReader(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		stub = testcase.Let(s, func(t *testcase.T) *iokit.StubReader {
			return &iokit.StubReader{Data: []byte(t.Random.Error().Error())}
		})
		keepAliveTime = testcase.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntBetween(int(time.Hour), int(24*time.Hour)))
		})
		subject = testcase.Let(s, func(t *testcase.T) io.ReadCloser {
			return iokit.NewKeepAliveReader(stub.Get(t), keepAliveTime.Get(t))
		})
	)

	s.Test("reading from the subject yields back all results from source reader", func(t *testcase.T) {
		got, err := io.ReadAll(subject.Get(t))
		assert.NoError(t, err)
		assert.Equal(t, got, stub.Get(t).Data)
	})

	s.When("reader is closed", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.NoError(t, subject.Get(t).Close())
		})

		s.Then("source reader is closed as well", func(t *testcase.T) {
			assert.True(t, stub.Get(t).IsClosed())
		})

		s.Then("triggering won't happen anymore", func(t *testcase.T) {
			assert.Empty(t, stub.Get(t).LastReadAt(), "something is not ok, the last read at at this point should be still empty")

			t.Random.Repeat(2, 5, func() {
				timecop.Travel(t, keepAliveTime.Get(t))
			})

			assert.Empty(t, stub.Get(t).LastReadAt(), "due to close, last read at should be still empty due to having the keep alive closed")
		})
	})

	s.When("there was no reading for the duration of keep alive time", func(s *testcase.Spec) {
		subject.EagerLoading(s)

		s.Before(func(t *testcase.T) {
			timecop.Travel(t, keepAliveTime.Get(t))
		})

		s.Then("the reader is still readable", func(t *testcase.T) {
			got, err := io.ReadAll(subject.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, got, stub.Get(t).Data)
		})

		s.Then("the keep alive mechanism read from the the source reader to avoid read timeout", func(t *testcase.T) {
			assert.Eventually(t, time.Second, func(it testing.TB) {
				assert.NotEmpty(it, stub.Get(t).LastReadAt)
			})

			t.Log("and even though the source reader is being read, the content can be still retrieved")
			got, err := io.ReadAll(subject.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, got, stub.Get(t).Data)
		})
	})

	s.When("source reader has an error on reading", func(s *testcase.Spec) {
		expErr := let.Error(s)

		stub.Let(s, func(t *testcase.T) *iokit.StubReader {
			r := stub.Super(t)
			r.ReadErr = expErr.Get(t)
			return r
		})

		s.Then("read error is propagated back", func(t *testcase.T) {
			_, err := io.ReadAll(subject.Get(t))
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.When("source reader has an error on closing", func(s *testcase.Spec) {
		expErr := let.Error(s)

		stub.Let(s, func(t *testcase.T) *iokit.StubReader {
			r := stub.Super(t)
			r.CloseErr = expErr.Get(t)
			return r
		})

		s.Then("read error is propagated back", func(t *testcase.T) {
			err := subject.Get(t).Close()
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.Test("calling Close multiple times should not be an issue", func(t *testcase.T) {
		t.Random.Repeat(2, 7, func() {
			assert.NoError(t, subject.Get(t).Close())
		})
	})
}

func TestPeekRune(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		r := &mockRuneReader{
			readRune:    'a',
			readSize:    1,
			unreadError: nil,
		}
		char, size, err := iokit.PeekRune(r)
		if char != 'a' || size != 1 || err != nil {
			t.Errorf("expected ('a', 1, nil), got (%q, %d, %v)", string(char), size, err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		r := &mockRuneReader{
			readError: errors.New("read failed"),
		}
		char, size, err := iokit.PeekRune(r)
		if char != 0 || size != 0 || err == nil {
			t.Errorf("expected (0, 0, error), got (%q, %d, %v)", string(char), size, err)
		}
	})

	t.Run("unread error", func(t *testing.T) {
		r := &mockRuneReader{
			readRune:    'a',
			readSize:    1,
			unreadError: errors.New("unread failed"),
		}
		char, size, err := iokit.PeekRune(r)
		if char != 'a' || size != 1 || err == nil {
			t.Errorf("expected ('a', 1, error), got (%q, %d, %v)", string(char), size, err)
		}
	})

	t.Run("smoke", func(t *testing.T) {
		t.Run("successful read", func(t *testing.T) {
			input := "abc"
			r := bytes.NewBufferString(input)
			reader := bufio.NewReader(r)
			char, size, err := iokit.PeekRune(reader)
			if char != 'a' || size != 1 || err != nil {
				t.Errorf("expected ('a', 1, nil), got (%q, %d, %v)", string(char), size, err)
			}
			// Check that the reader is still at the beginning
			char, size, err = iokit.PeekRune(reader)
			if char != 'a' || size != 1 || err != nil {
				t.Errorf("expected ('a', 1, nil), got (%q, %d, %v)", string(char), size, err)
			}

			bs, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, input, string(bs))
		})

		t.Run("read error", func(t *testing.T) {
			r := bytes.NewBuffer([]byte{})
			reader := bufio.NewReader(r)
			char, size, err := iokit.PeekRune(reader)
			if char != 0 || size != 0 || err == nil {
				t.Errorf("expected (0, 0, error), got (%q, %d, %v)", string(char), size, err)
			}
			assert.ErrorIs(t, err, io.EOF)
		})
	})
}

type mockRuneReader struct {
	readRune    rune
	readSize    int
	readError   error
	unreadError error
}

func (m *mockRuneReader) ReadRune() (rune, int, error) {
	return m.readRune, m.readSize, m.readError
}

func (m *mockRuneReader) UnreadRune() error {
	return m.unreadError
}

func TestPeekByte(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		r := &mockByteReader{
			readByte:    0x12,
			readError:   nil,
			unreadError: nil,
		}
		b, err := iokit.PeekByte(r)
		if b != 0x12 || err != nil {
			t.Errorf("expected (0x12, nil), got (0x%x, %v)", b, err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		r := &mockByteReader{
			readError: errors.New("read failed"),
		}
		b, err := iokit.PeekByte(r)
		if b != 0 || err == nil {
			t.Errorf("expected (0, error), got (0x%x, %v)", b, err)
		}
	})

	t.Run("unread error", func(t *testing.T) {
		r := &mockByteReader{
			readByte:    0x12,
			unreadError: errors.New("unread failed"),
		}
		b, err := iokit.PeekByte(r)
		if b != 0x12 || err == nil {
			t.Errorf("expected (0x12, error), got (0x%x, %v)", b, err)
		}
	})

	t.Run("smoke", func(t *testing.T) {
		input := "abc"
		r := bytes.NewBufferString(input)
		reader := bufio.NewReader(r)
		b, err := iokit.PeekByte(reader)
		if b != 'a' || err != nil {
			t.Errorf("expected ('a', nil), got (0x%x, %v)", b, err)
		}
		// Check that the reader is still at the beginning
		b, err = iokit.PeekByte(reader)
		if b != 'a' || err != nil {
			t.Errorf("expected ('a', nil), got (0x%x, %v)", b, err)
		}

		bs, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.Equal(t, input, string(bs))
	})
}

type mockByteReader struct {
	readByte    byte
	readError   error
	unreadError error
}

func (m *mockByteReader) ReadByte() (byte, error) {
	return m.readByte, m.readError
}

func (m *mockByteReader) UnreadByte() error {
	return m.unreadError
}

func TestMoveByte(t *testing.T) {
	t.Run("successful move", func(t *testing.T) {
		bs := []byte("abc")
		in := bytes.NewReader(bs)
		out := &bytes.Buffer{}
		b, err := iokit.MoveByte(in, out)
		assert.NoError(t, err)
		assert.Equal(t, bs[0], b)
		assert.Equal(t, []byte("bc"), assert.ReadAll(t, in))
	})

	t.Run("read error", func(t *testing.T) {
		in := &mockByteReader{
			readError: errors.New("read failed"),
		}
		out := bytes.NewBuffer(nil)
		b, err := iokit.MoveByte(in, out)
		if b != 0 || err == nil {
			t.Errorf("expected (0, error), got (0x%x, %v)", b, err)
		}
		if out.Len() != 0 {
			t.Errorf("expected written byte to be empty, but was 0x%x", out.Bytes())
		}
	})

	t.Run("write error", func(t *testing.T) {
		in := &mockByteReader{
			readByte:  0x12,
			readError: nil,
		}
		out := &errorWriter{}
		b, err := iokit.MoveByte(in, out)
		if b != 0 || err == nil {
			t.Errorf("expected (0, error), got (0x%x, %v)", b, err)
		}
	})

	t.Run("smoke", func(t *testing.T) {
		input := "abc"
		in := bytes.NewBufferString(input)
		out := &bytes.Buffer{}
		b, err := iokit.MoveByte(in, out)
		if b != 'a' || err != nil {
			t.Errorf("expected ('a', nil), got (0x%x, %v)", b, err)
		}
		assert.Equal(t, "a", string(assert.ReadAll(t, out)))
	})
}

type errorWriter struct{}

func (e *errorWriter) WriteByte(c byte) error {
	return errors.New("write failed")
}

func TestMoveRune(t *testing.T) {
	t.Run("successful move", func(t *testing.T) {
		r := &mockRuneReaderWriter{
			readRune:   'a',
			readSize:   1,
			readError:  nil,
			writeError: nil,
		}
		char, size, err := iokit.MoveRune(r, r)
		if char != 'a' || size != 1 || err != nil {
			t.Errorf("expected ('a', 1, nil), got (%q, %d, %v)", string(char), size, err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		r := &mockRuneReaderWriter{
			readError: errors.New("read failed"),
		}
		char, size, err := iokit.MoveRune(r, r)
		if char != 0 || size != 0 || err == nil {
			t.Errorf("expected (0, 0, error), got (%q, %d, %v)", string(char), size, err)
		}
	})

	t.Run("write error", func(t *testing.T) {
		r := &mockRuneReaderWriter{
			readRune:   'a',
			readSize:   1,
			writeError: errors.New("write failed"),
		}
		out := &bytes.Buffer{}
		char, size, err := iokit.MoveRune(r, out)
		assert.NoError(t, err)
		assert.Equal(t, size, 1)
		assert.Equal(t, char, 'a')
	})

	t.Run("smoke", func(t *testing.T) {
		input := "abc"
		r := bytes.NewBufferString(input)
		reader := bufio.NewReader(r)
		writer := &bytes.Buffer{}

		char, size, err := iokit.MoveRune(reader, writer)
		assert.NoError(t, err)
		assert.NotEmpty(t, size)
		assert.Equal(t, 'a', char)
		assert.Equal(t, "a", string(assert.ReadAll(t, writer)))
		assert.Equal(t, "bc", string(assert.ReadAll(t, reader)))
	})
}

type mockRuneReaderWriter struct {
	readRune    rune
	readSize    int
	readError   error
	writeError  error
	writtenData []rune
}

func (m *mockRuneReaderWriter) ReadRune() (rune, int, error) {
	return m.readRune, m.readSize, m.readError
}

func (m *mockRuneReaderWriter) UnreadRune() error {
	// Not used in MoveRune
	return nil
}

func (m *mockRuneReaderWriter) WriteRune(r rune) (n int, err error) {
	m.writtenData = append(m.writtenData, r)
	if m.writeError != nil {
		return 0, m.writeError
	}
	return 1, nil
}
