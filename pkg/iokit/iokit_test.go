package iokit_test

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
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

func TestWriteAll(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		stub = let.Var(s, func(t *testcase.T) *iokit.Stub {
			return &iokit.Stub{}
		})
		p = let.Var(s, func(t *testcase.T) []byte {
			var data = make([]byte, t.Random.IntBetween(1, 42))
			_, err := io.ReadFull(t.Random, data)
			assert.NoError(t, err)
			return data
		})
	)
	act := let.Act2(func(t *testcase.T) (int, error) {
		return iokit.WriteAll(stub.Get(t), p.Get(t))
	})

	s.Then("it will write all data into the writer", func(t *testcase.T) {
		n, err := act(t)
		assert.Equal(t, len(p.Get(t)), n)
		assert.NoError(t, err)
		assert.Equal(t, p.Get(t), stub.Get(t).Bytes())
	})

	s.When("partial/short write occurs during writing", func(s *testcase.Spec) {
		p.Let(s, func(t *testcase.T) []byte {
			t.Log("given we have a long enough byte slice that contains more than a single byte")
			var data = make([]byte, t.Random.IntBetween(13, 1000))
			_, err := io.ReadFull(t.Random, data)
			assert.NoError(t, err)
			return data
		})

		stub.Let(s, func(t *testcase.T) *iokit.Stub {
			var chunks = t.Random.IntBetween(1, len(p.Get(t))/2)
			return &iokit.Stub{
				StubWrite: func(stub *iokit.Stub, p []byte) (int, error) {
					if 0 < chunks {
						chunks--
						n, err := stub.Write(p[:1])
						assert.NoError(t, err, "expected no error from stub read")
						if t.Random.Bool() { // WriteAll should handle both case, when we have and when we don't have ShortWrite error
							err = io.ErrShortWrite
						}
						return n, err
					}
					return stub.Write(p)
				},
			}
		})

		s.Then("it will write everything onto the io.Writer", func(t *testcase.T) {
			n, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, n, len(p.Get(t)))
			assert.Equal(t, p.Get(t), stub.Get(t).Data)
		})
	})
}

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
			assert.Must(t).NoError(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data.Get(t))
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(len(data.Get(t)), n)
			assert.Must(t).NoError(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			assert.Must(t).NoError(err)
			t.Defer(f.Close)
			return f
		}}
	)

	thenContentsAreMatching := func(s *testcase.Spec) {
		s.Then("contents are matching with the reference ReadWriteSeeker", func(t *testcase.T) {
			expected, err := io.ReadAll(rwsc.Get(t))
			assert.Must(t).NoError(err)
			actual, err := io.ReadAll(buffer.Get(t))
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(string(expected), string(actual))
		})
	}

	thenContentsAreMatching(s)

	seekOnBoth := func(t *testcase.T, offset int64, whench int) {
		expected, err := rwsc.Get(t).Seek(offset, whench)
		assert.Must(t).NoError(err)
		actual, err := buffer.Get(t).Seek(offset, whench)
		assert.Must(t).NoError(err)
		assert.Must(t).Equal(expected, actual, "seek length matches")
	}

	writeOnBoth := func(t *testcase.T, bs []byte) {
		expected, err := rwsc.Get(t).Write(bs)
		assert.Must(t).NoError(err)
		actual, err := buffer.Get(t).Write(bs)
		assert.Must(t).NoError(err)
		assert.Must(t).Equal(expected, actual, "write length matches")
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
				assert.True(t, errors.As(err, &pathError))
				_, err = buffer.Get(t).Seek(offset.Get(t), whench.Get(t))
				assert.Must(t).ErrorIs(iokit.ErrSeekNegativePosition, err)
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
			assert.Must(t).NoError(buffer.Get(t).Close())
			assert.Must(t).NoError(rwsc.Get(t).Close())
		})

		s.Then("simultaneous .Close yields error", func(t *testcase.T) {
			err := rwsc.Get(t).Close()
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
			err = buffer.Get(t).Close()
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
		})

		s.Then(".Read fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Read([]byte{})
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Read([]byte{})
			assert.Must(t).ErrorIs(iokit.ErrClosed, err)
		})

		s.Then(".Write fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Write([]byte{})
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Write([]byte{})
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
		})

		s.Then(".Seek fails with fs.ErrClosed", func(t *testcase.T) {
			_, err := rwsc.Get(t).Seek(0, io.SeekStart)
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
			_, err = buffer.Get(t).Seek(0, io.SeekStart)
			assert.Must(t).ErrorIs(fs.ErrClosed, err)
		})
	})

	s.Describe(".String and .Bytes", func(s *testcase.Spec) {
		thenStringAndBytesResultsAreMatchesTheRefRWSCReadContent := func(s *testcase.Spec) {
			thenContentsAreMatching(s)

			s.Then(".String() matches with reference rwsc's read content", func(t *testcase.T) {
				rwsc.Get(t).Seek(0, io.SeekStart)
				expectedBS, err := io.ReadAll(rwsc.Get(t))
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(string(expectedBS), buffer.Get(t).String())
				assert.Must(t).Equal(expectedBS, buffer.Get(t).Bytes())
			})

			s.Then(".Bytes() matches with reference rwsc's read content", func(t *testcase.T) {
				rwsc.Get(t).Seek(0, io.SeekStart)
				expectedBS, err := io.ReadAll(rwsc.Get(t))
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(string(expectedBS), buffer.Get(t).String())
				assert.Must(t).Equal(expectedBS, buffer.Get(t).Bytes())
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
			assert.Must(t).NoError(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(len(data), n)
			assert.Must(t).NoError(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			assert.Must(t).NoError(err)
			t.Defer(f.Close)
			return f
		}(t)
	)

	bseek, err := buffer.Seek(offset, whench)
	assert.Must(t).NoError(err)

	fseek, err := file.Seek(offset, whench)
	assert.Must(t).NoError(err)

	assert.Must(t).Equal(fseek, bseek)

	wData := []byte(t.Random.StringN(1))
	bwrite, err := buffer.Write(wData)
	assert.Must(t).NoError(err)
	fwrite, err := file.Write(wData)
	assert.Must(t).NoError(err)
	assert.Must(t).Equal(fwrite, bwrite)

	_, err = file.Seek(0, io.SeekStart)
	assert.Must(t).NoError(err)
	_, err = buffer.Seek(0, io.SeekStart)
	assert.Must(t).NoError(err)

	expectedBS, err := io.ReadAll(file)
	assert.Must(t).NoError(err)
	actualBS, err := io.ReadAll(buffer)
	assert.Must(t).NoError(err)
	assert.Must(t).Equal(string(expectedBS), string(actualBS))
}

func TestNew_smoke(tt *testing.T) {
	t := testcase.NewT(tt)
	dataSTR := t.Random.String()
	dataBS := []byte(t.Random.String())
	assert.Must(t).Equal(iokit.NewBuffer(dataSTR).String(), dataSTR)
	assert.Must(t).Equal(iokit.NewBuffer(dataBS).Bytes(), dataBS)
}

func TestBuffer_Read_ioReadAll(t *testing.T) {
	t.Run("on empty buffer", func(tt *testing.T) {
		t := testcase.NewT(tt)
		b := &iokit.Buffer{}
		bs, err := io.ReadAll(b)
		assert.Must(t).NoError(err)
		assert.Must(t).Empty(bs)
	})
	t.Run("on populated buffer", func(tt *testing.T) {
		t := testcase.NewT(tt)
		d := t.Random.String()
		b := iokit.NewBuffer(d)
		bs, err := io.ReadAll(b)
		assert.Must(t).NoError(err)
		assert.Must(t).Equal(d, string(bs))
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
		body := &iokit.Stub{
			Data: []byte("foo"),
			StubRead: func(stub *iokit.Stub, p []byte) (int, error) {
				return 0, expErr
			},
		}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.ErrorIs(t, err, expErr)
		assert.Empty(t, data)
	})
	t.Run("Closer type is closed", func(t *testing.T) {
		body := &iokit.Stub{Data: []byte("foo")}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.NoError(t, err)
		assert.Equal(t, body.Data, data)
		assert.True(t, body.IsClosed())
	})
	t.Run("Closing error is propagated", func(t *testing.T) {
		expErr := rnd.Error()
		body := &iokit.Stub{
			Data: []byte("foo"),
			StubClose: func(stub *iokit.Stub) error {
				return expErr
			},
		}
		data, err := iokit.ReadAllWithLimit(body, 50*iokit.Megabyte)
		assert.ErrorIs(t, err, expErr)
		assert.Equal(t, data, body.Data)
	})
}

func TestStub(t *testing.T) {
	s := testcase.NewSpec(t)

	stub := let.Var(s, func(t *testcase.T) *iokit.Stub {
		return &iokit.Stub{}
	})

	s.Describe("#Read", func(s *testcase.Spec) {
		s.Test("behaves like bytes.Reader", func(t *testcase.T) {
			data := []byte(t.Random.String())

			stubReader := &iokit.Stub{Data: data}
			refReader1 := bytes.NewReader(data)
			refReader2 := bytes.NewBuffer(data)

			var (
				expectedN1, expectedN2, gotN       int
				expectedErr1, expectedErr2, gotErr error
				buf1, buf2, buf3                   []byte
			)
			t.Log("on reading half of the content")
			var half = len(data) / 2
			buf1 = make([]byte, half)
			buf2 = make([]byte, half)
			buf3 = make([]byte, half)
			expectedN1, expectedErr1 = refReader1.Read(buf1)
			expectedN2, expectedErr2 = refReader2.Read(buf2)
			gotN, gotErr = stubReader.Read(buf3)
			assert.Equal(t, expectedN1, expectedN2)
			assert.Equal(t, expectedErr1, expectedErr2)
			assert.Equal(t, expectedErr1, gotErr)
			assert.Equal(t, expectedN1, gotN)
			assert.Equal(t, buf1, buf2)
			assert.Equal(t, buf1, buf3)

			t.Log("remaining more than the remaining")
			var moreThanRemainder = len(data) + 1
			buf1 = make([]byte, moreThanRemainder)
			buf2 = make([]byte, moreThanRemainder)
			buf3 = make([]byte, moreThanRemainder)
			expectedN1, expectedErr1 = refReader1.Read(buf1)
			expectedN2, expectedErr2 = refReader2.Read(buf2)
			gotN, gotErr = stubReader.Read(buf3)
			assert.Equal(t, expectedN1, expectedN2)
			assert.Equal(t, expectedErr1, expectedErr2)
			assert.Equal(t, expectedErr1, gotErr)
			assert.Equal(t, expectedN1, gotN)
			assert.Equal(t, buf1, buf2)
			assert.Equal(t, buf1, buf3)

			t.Log("on reading an already exhausted reader")
			buf1 = make([]byte, 1)
			buf2 = make([]byte, 1)
			buf3 = make([]byte, 1)
			expectedN1, expectedErr1 = refReader1.Read(buf1)
			expectedN2, expectedErr2 = refReader2.Read(buf2)
			gotN, gotErr = stubReader.Read(buf3)
			assert.Equal(t, expectedN1, expectedN2)
			assert.Equal(t, expectedErr1, expectedErr2)
			assert.Equal(t, expectedErr1, gotErr)
			assert.Equal(t, expectedN1, gotN)
		})

		s.Test("reads data exact size", func(t *testcase.T) {
			data := []byte(t.Random.String())
			stub := &iokit.Stub{Data: data}
			buf1 := make([]byte, len(data))
			n, err := stub.Read(buf1)
			assert.NoError(t, err)
			assert.Equal(t, len(data), n)
			assert.Equal(t, buf1, data)
		})

		s.Test("allows the injection of a Read error", func(t *testcase.T) {
			expErr := t.Random.Error()
			reader := &iokit.Stub{StubRead: func(stub *iokit.Stub, p []byte) (int, error) {
				return 0, expErr
			}}

			buf := make([]byte, 1)
			_, err := reader.Read(buf)
			assert.ErrorIs(t, err, expErr)
		})

		s.Test("with EarlyEOF, it returns EOF on read past end of data", func(t *testcase.T) {
			reader := &iokit.Stub{
				Data:     []byte(t.Random.String()),
				EagerEOF: true,
			}

			buf := make([]byte, len(reader.Data)*2)
			readLen, err := reader.Read(buf)
			assert.ErrorIs(t, err, io.EOF)
			assert.Equal(t, len(reader.Data), readLen)
			assert.Equal(t, buf[:len(reader.Data)], reader.Data)
		})

		s.Test("without EarlyEOF, it will not return EOF on read past end of data", func(t *testcase.T) {
			reader := &iokit.Stub{
				Data:     []byte(t.Random.String()),
				EagerEOF: false,
			}

			buf := make([]byte, len(reader.Data)*2)
			readLen, err := reader.Read(buf)
			assert.NoError(t, err)
			assert.Equal(t, len(reader.Data), readLen)
			assert.Equal(t, buf[:len(reader.Data)], reader.Data)

			readLen, err = reader.Read(make([]byte, 1))
			assert.ErrorIs(t, err, io.EOF)
			assert.Equal(t, readLen, 0)
		})

		s.Test("closes without an error", func(t *testcase.T) {
			reader := &iokit.Stub{StubClose: func(stub *iokit.Stub) error {
				return stub.Close()
			}}
			assert.NoError(t, reader.Close())
			assert.True(t, reader.IsClosed())
		})

		s.Test("safe to close multiple times", func(t *testcase.T) {
			reader := &iokit.Stub{}
			t.Random.Repeat(2, 5, func() {
				assert.NoError(t, reader.Close())
			})
		})

		s.Test("allows the injection of a Close error", func(t *testcase.T) {
			expErr := errors.New("test error")
			reader := &iokit.Stub{StubClose: func(stub *iokit.Stub) error {
				return expErr
			}}
			err := reader.Close()
			assert.ErrorIs(t, expErr, err)
		})

		s.Test("we can determine when the reading will update the LastReadAt's result", func(t *testcase.T) {
			now := clock.Now()
			timecop.Travel(t, now, timecop.Freeze)

			data := []byte(t.Random.StringN(5))
			stub := &iokit.Stub{Data: data}
			assert.Empty(t, stub.NumRead())

			n, err := stub.Read(make([]byte, 1))
			assert.NoError(t, err)
			assert.Equal(t, 1, n)
			assert.Equal(t, stub.NumRead(), 1)

			n, err = stub.Read(make([]byte, 1))
			assert.NoError(t, err)
			assert.Equal(t, 1, n)
			assert.Equal(t, stub.NumRead(), 2)
		})

		s.Test("NumRead is thread safe", func(t *testcase.T) {
			stub := &iokit.Stub{Data: []byte(t.Random.StringN(5))}

			testcase.Race(func() {
				stub.NumRead()
			}, func() {
				stub.NumRead()
			}, func() {
				_, _ = stub.Read(make([]byte, 1))
			})
		})

		s.Test("IsClosed is thread safe", func(t *testcase.T) {
			stub := &iokit.Stub{Data: []byte(t.Random.StringN(5))}

			testcase.Race(func() {
				_ = stub.IsClosed()
			}, func() {
				_ = stub.Close()
			})
		})

		s.Test("after closing the reader, LastReadAt still records reading attempts", func(t *testcase.T) {
			stub := &iokit.Stub{Data: []byte(t.Random.StringN(5))}
			assert.NoError(t, stub.Close())
			_, _ = stub.Read(make([]byte, 1))
			assert.NotEmpty(t, stub.NumRead())
		})
	})

	s.Describe("#Write", func(s *testcase.Spec) {
		var (
			p = let.Var(s, func(t *testcase.T) []byte {
				var data = make([]byte, t.Random.IntBetween(2, 128))
				_, err := io.ReadFull(t.Random, data)
				assert.NoError(t, err)
				return data
			})
		)
		act := let.Act2(func(t *testcase.T) (int, error) {
			return stub.Get(t).Write(p.Get(t))
		})

		s.Then("it will record the written data", func(t *testcase.T) {
			n, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, n, len(p.Get(t)))
			assert.Equal(t, stub.Get(t).Data, p.Get(t))
		})

		s.When("write error is configured", func(s *testcase.Spec) {
			expErr := let.Error(s)
			stub.Let(s, func(t *testcase.T) *iokit.Stub {
				v := stub.Super(t)
				v.StubWrite = func(stub *iokit.Stub, p []byte) (int, error) {
					return 0, expErr.Get(t)
				}
				return v
			})

			s.Then("error is returned back", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, expErr.Get(t))
			})
		})

		s.Test("on repeated write, all writes' data are appended to the stub", func(t *testcase.T) {
			_, err := stub.Get(t).Write([]byte("foo"))
			assert.NoError(t, err)
			_, err = stub.Get(t).Write([]byte("bar"))
			assert.NoError(t, err)
			_, err = stub.Get(t).Write([]byte("baz"))
			assert.NoError(t, err)
			assert.Equal(t, []byte("foobarbaz"), stub.Get(t).Data)
		})
	})

	s.Context("Stub function fields", func(s *testcase.Spec) {
		s.Test("act as a middleware and calling original method will not cause inf recursion", func(t *testcase.T) {
			var exp = make([]byte, t.Random.IntBetween(1, 42))
			t.Random.Read(exp)

			var writeOK, readOK, closeOK bool
			stub := &iokit.Stub{
				Data: nil,
				StubWrite: func(stub *iokit.Stub, p []byte) (int, error) {
					writeOK = true
					return stub.Write(p)
				},
				StubRead: func(stub *iokit.Stub, p []byte) (int, error) {
					readOK = true
					return stub.Read(p)
				},
				StubClose: func(stub *iokit.Stub) error {
					closeOK = true
					return stub.Close()
				},
			}

			n, err := stub.Write(exp)
			assert.NoError(t, err)
			assert.Equal(t, n, len(exp))
			assert.True(t, writeOK)
			assert.Equal(t, stub.NumWrite(), 1)

			var got = make([]byte, len(exp))
			n, err = stub.Read(got)
			assert.NoError(t, err)
			assert.Equal(t, n, len(exp))
			assert.Equal(t, exp, got)
			assert.True(t, readOK)
			assert.Equal(t, stub.NumRead(), 1)

			assert.NoError(t, stub.Close())
			assert.True(t, stub.IsClosed())
			assert.True(t, closeOK)
		})

		s.Test("Stub can intercept/affect the returned values", func(t *testcase.T) {
			var exp = make([]byte, t.Random.IntBetween(1, 42))
			t.Random.Read(exp)

			var (
				wN   = t.Random.Int()
				wErr = t.Random.Error()
				rN   = t.Random.Int()
				rErr = t.Random.Error()
				cErr = t.Random.Error()
			)
			stub := &iokit.Stub{
				Data: nil,
				StubWrite: func(stub *iokit.Stub, p []byte) (int, error) {
					return wN, wErr
				},
				StubRead: func(stub *iokit.Stub, p []byte) (int, error) {
					return rN, rErr
				},
				StubClose: func(stub *iokit.Stub) error {
					return cErr
				},
			}

			n, err := stub.Write(exp)
			assert.Equal(t, wN, n)
			assert.Equal(t, wErr, err)
			n, err = stub.Read(make([]byte, 1))
			assert.Equal(t, rN, n)
			assert.Equal(t, rErr, err)
			assert.Equal(t, stub.Close(), cErr)
		})
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
		stub = testcase.Let(s, func(t *testcase.T) *iokit.Stub {
			return &iokit.Stub{Data: []byte(t.Random.Error().Error())}
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
			assert.Empty(t, stub.Get(t).NumRead(), "something is not ok, the last read at at this point should be still empty")

			t.Random.Repeat(2, 5, func() {
				timecop.Travel(t, keepAliveTime.Get(t))
			})

			assert.Empty(t, stub.Get(t).NumRead(), "due to close, last read at should be still empty due to having the keep alive closed")
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
				assert.NotEmpty(it, stub.Get(t).NumRead())
			})

			t.Log("and even though the source reader is being read, the content can be still retrieved")
			got, err := io.ReadAll(subject.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, got, stub.Get(t).Data)
		})
	})

	s.When("source reader has an error on reading", func(s *testcase.Spec) {
		expErr := let.Error(s)

		stub.Let(s, func(t *testcase.T) *iokit.Stub {
			r := stub.Super(t)
			r.StubRead = func(stub *iokit.Stub, p []byte) (int, error) {
				return 0, expErr.Get(t)
			}
			return r
		})

		s.Then("read error is propagated back", func(t *testcase.T) {
			_, err := io.ReadAll(subject.Get(t))
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.When("source reader has an error on closing", func(s *testcase.Spec) {
		expErr := let.Error(s)

		stub.Let(s, func(t *testcase.T) *iokit.Stub {
			r := stub.Super(t)
			r.StubClose = func(stub *iokit.Stub) error {
				return expErr.Get(t)
			}
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

func ExampleLockstepReaders() {
	input := strings.NewReader(strings.Repeat("EndIsNeverThe", 1024))

	rs := iokit.LockstepReaders(input, 3, 16*iokit.Megabyte)

	var g synckit.Group
	defer g.Wait()

	for i := range rs {
		r := rs[i]
		g.Go(nil, func(ctx context.Context) error {
			data, err := io.ReadAll(r)
			_ = data
			return err
		})
	}
}

func TestLockstepReaders(t *testing.T) {
	s := testcase.NewSpec(t)

	var MakeRandomData = func(t *testcase.T, length int) []byte {
		var data = make([]byte, length)
		_, err := t.Random.Read(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), length)
		return data
	}

	var (
		bufferWindowSize = let.Var(s, func(t *testcase.T) iokit.ByteSize {
			return t.Random.IntBetween(iokit.Byte, iokit.Kilobyte)
		})
		data = let.Var(s, func(t *testcase.T) []byte {
			mul := t.Random.IntBetween(2, 8)
			length := t.Random.IntBetween(bufferWindowSize.Get(t), bufferWindowSize.Get(t)*mul)
			return MakeRandomData(t, length)
		})
		sourceReader = let.Var(s, func(t *testcase.T) io.Reader {
			return bytes.NewReader(data.Get(t))
		})
		count = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntBetween(1, 7)
		})
	)

	act := let.Act(func(t *testcase.T) []io.ReadCloser {
		rs := iokit.LockstepReaders(sourceReader.Get(t), count.Get(t), bufferWindowSize.Get(t))
		assert.Equal(t, len(rs), count.Get(t))
		return rs
	})

	type BGRead struct {
		synckit.Group
		Results synckit.Slice[[]byte]
	}
	var readAll = func(t *testcase.T, readers []io.ReadCloser) *BGRead {
		var bgr BGRead
		t.Cleanup(bgr.Cancel)
		t.Cleanup(func() {
			bgr.Cancel()
			t.Eventually(func(t *testcase.T) {
				assert.Empty(t, bgr.Group.Len())
			})
		})
		for _, r := range readers {
			var r = r
			bgr.Group.Go(func(ctx context.Context) error {
				data, err := io.ReadAll(r)
				if err == nil {
					bgr.Results.Append(data)
				}
				return err
			})
		}
		return &bgr
	}

	var ThenTheOutputReadersWillContainTheSameData = func(s *testcase.Spec) {
		s.Then("the lockstep reader(s) will be able to read the same identical data from the source reader", func(t *testcase.T) {
			readers := act(t)

			bgr := readAll(t, readers)

			assert.Within(t, time.Second, func(ctx context.Context) {
				assert.NoError(t, bgr.Wait())
			}, "all the lockstep reader should finished through concurrent reading")

			assert.Equal(t, count.Get(t), bgr.Results.Len(),
				"all the reader should have finished without an error")

			for got := range bgr.Results.Iter() {
				assert.Equal(t, data.Get(t), got, "all the output results should be identical")
			}
		})
	}

	s.Context("incorrect usage", func(s *testcase.Spec) {
		s.When("negative output reader count requested", func(s *testcase.Spec) {
			count.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(-1, -100)
			})

			s.Then("it is considered as a programming error and it will panic", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })
			})
		})

		s.When("negative buffer window is defined", func(s *testcase.Spec) {
			bufferWindowSize.Let(s, func(t *testcase.T) int {
				return t.Random.IntBetween(-1, -100)
			})

			s.Then("it is considered as a programming error and it will panic", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })
			})
		})

		s.When("nil source reader is provided", func(s *testcase.Spec) {
			sourceReader.LetValue(s, nil)

			s.Then("it is considered as a programming error and it will panic", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })
			})
		})
	})

	s.When("zero output reader is requested", func(s *testcase.Spec) {
		count.LetValue(s, 0)

		s.Then("empty reader result received", func(t *testcase.T) {
			assert.Empty(t, act(t))
		})
	})

	s.When("the reader count is one", func(s *testcase.Spec) {
		count.LetValue(s, 1)

		s.Then("it will create a single output reader", func(t *testcase.T) {
			readers := act(t)
			assert.Equal(t, len(readers), count.Get(t))
		})

		ThenTheOutputReadersWillContainTheSameData(s)
	})

	ThenTheOutputReadersWillContainTheSameData(s)

	s.Context("sync", func(s *testcase.Spec) {
		count.Let(s, func(t *testcase.T) int {
			t.Log("in sync aspects related testing we work with at least two concurrent lockstep reader")
			return t.Random.IntBetween(2, 7)
		})

		data.Let(s, func(t *testcase.T) []byte {
			t.Log("in sync related tests, the source reader's total data must be larger than the buffering window byte size")
			mul := t.Random.IntBetween(2, 8)
			length := t.Random.IntBetween(bufferWindowSize.Get(t)+1, bufferWindowSize.Get(t)*mul)
			return MakeRandomData(t, length)
		})

		s.Test("if a lockstep reader laggs behind, then the others will have to wait", func(t *testcase.T) {
			readers := act(t)

			otherReaders := readers[1:]
			bg := readAll(t, otherReaders)

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bg.Len(), len(otherReaders), "expected that other readers eventually queu up and start to waiting")
			})
			for range t.Random.IntBetween(3, 7) {
				runtime.Gosched()
				assert.Equal(t, bg.Len(), len(otherReaders), "queud up lockstep readers should be still waiting")
			}

			first, ok := slicekit.First(readers)
			assert.True(t, ok)
			defer first.Close()

			assert.Within(t, time.Second, func(ctx context.Context) {
				n, err := first.Read(make([]byte, 1))
				assert.NoError(t, err)
				assert.Equal(t, 1, n)
			})

			for range t.Random.IntBetween(13, 42) {
				runtime.Gosched()
				assert.Equal(t, bg.Len(), len(otherReaders), "other readers should be still stuck waiting")
			}

			for range t.Random.IntBetween(3, 42) {
				runtime.Gosched()
				assert.Empty(t, bg.Results.Len())
			}

			t.Log("star reading the first consumer")
			assert.Within(t, time.Second, func(ctx context.Context) {
				_, err := io.ReadAll(first)
				assert.NoError(t, err)
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bg.Len(), 0)
			})

			for got := range bg.Results.Iter() {
				assert.Equal(t, data.Get(t), got)
			}
		})

		s.Test("reading one buffer window size should be always possible without being blocked", func(t *testcase.T) {
			readers := act(t)

			otherReaders := readers[1:]
			bg := readAll(t, otherReaders)

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bg.Len(), len(otherReaders))
			})

			first, ok := slicekit.First(readers)
			assert.True(t, ok)
			defer first.Close()

			assert.Within(t, time.Second, func(ctx context.Context) {
				n, err := first.Read(make([]byte, bufferWindowSize.Get(t)))
				assert.NoError(t, err)
				assert.Equal(t, n, bufferWindowSize.Get(t))
			})
		})

		s.Test("closing a lockstep reader removes it from the active readers group", func(t *testcase.T) {
			readers := act(t)

			otherReaders := readers[1:]
			bg := readAll(t, otherReaders)

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, bg.Len(), len(otherReaders))
			})

			first, ok := slicekit.First(readers)
			assert.True(t, ok)
			defer first.Close()

			assert.Within(t, time.Second, func(ctx context.Context) {
				n, err := first.Read(make([]byte, 1))
				assert.NoError(t, err)
				assert.Equal(t, 1, n)
			})

			assert.NoError(t, first.Close())

			t.Eventually(func(t *testcase.T) {
				assert.Empty(t, bg.Len())
				assert.Equal(t, len(otherReaders), bg.Results.Len())
			})

			for got := range bg.Results.Iter() {
				assert.Equal(t, data.Get(t), got)
			}
		})
	})

	s.Test("close invalidates the reader from the group", func(t *testcase.T) {
		var (
			data = []byte(t.Random.String())
			lsrs = iokit.LockstepReaders(bytes.NewReader(data), 2, len(data)/3+1)
			g    synckit.Group
		)

		g.Go(nil, func(ctx context.Context) error {
			defer lsrs[0].Close()
			return nil
		})

		var gotData []byte
		g.Go(nil, func(ctx context.Context) error {
			defer lsrs[1].Close()
			data, err := io.ReadAll(lsrs[1])
			gotData = data
			return err
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, g.Wait())
		})

		assert.Equal(t, data, gotData)

	})

	s.Test("close a the reader of a single instance lsreader group invalidates the whole group", func(t *testcase.T) {
		var (
			data = []byte(t.Random.String())
			lsrs = iokit.LockstepReaders(bytes.NewReader(data), 1, len(data)/3+1)
			g    synckit.Group
		)

		assert.Equal(t, len(lsrs), 1)

		g.Go(nil, func(ctx context.Context) error {
			return lsrs[0].Close()
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, g.Wait())
		})
	})

	s.Test("closing a lockstep reader multiple times should yield no error", func(t *testcase.T) {
		readers := act(t)

		for _, r := range readers {
			t.Random.Repeat(3, 7, func() {
				assert.NoError(t, r.Close())
			})
		}
	})

	s.Test("race", func(t *testcase.T) {
		rs := iokit.LockstepReaders(sourceReader.Get(t), 3, 1)

		a, b, c := rs[0], rs[1], rs[2]
		testcase.Race(func() {
			io.ReadAll(a)
		}, func() {
			time.Sleep(time.Millisecond)
			runtime.Gosched()
			b.Close()
		}, func() {
			defer c.Close()
			io.ReadFull(c, make([]byte, len(data.Get(t))/2))
		})
	})

	s.Test("pprof", func(t *testcase.T) {
		testcase.GetEnv(t, "profile", t.SkipNow)
		const DataMoveSize = 100 * iokit.Megabyte
		var window = cmp.Or(DataMoveSize/100, DataMoveSize/10, 1)
		_ = window

		var g synckit.Group
		t.Cleanup(func() { assert.NoError(t, g.Wait()) })
		out, in := io.Pipe()

		g.Go(nil, func(ctx context.Context) error {
			defer in.Close()

			chunk := make([]byte, window)
			t.Random.Read(chunk)

			var written int
			for written < DataMoveSize {
				var toWrite = chunk
				if DataMoveSize < len(toWrite)+window {
					toWrite = toWrite[:DataMoveSize-window]
				}
				n, err := in.Write(toWrite)
				if err != nil {
					return fmt.Errorf("write error: %w", err)
				}
				written += n
			}
			return in.Close()
		})

		for _, r := range iokit.LockstepReaders(out, 3, window) {
			r := r
			g.Go(nil, func(ctx context.Context) error {
				var p = make([]byte, 10)
				for {
					_, err := r.Read(p)
					if err != nil {
						if errors.Is(err, io.EOF) {
							return nil
						}
						return err
					}
				}
			})
		}
	})
}

func BenchmarkLockstepReaders(b *testing.B) {
	rnd := random.New(random.CryptoSeed{})

	const sampling = 3
	b.Run("100KB", func(b *testing.B) {
		window := iokit.Kilobyte
		input := rnd.StringN(100 * window)

		b.Run("1", func(b *testing.B) {
			for b.Loop() {
				lrs := iokit.LockstepReaders(strings.NewReader(input), 1, window)
				io.ReadAll(lrs[0])
			}
		})
		b.Run("N", func(b *testing.B) {
			var g synckit.Group
			for b.Loop() {
				lrs := iokit.LockstepReaders(strings.NewReader(input), sampling, window)
				for _, r := range lrs {
					var r = r
					g.Go(nil, func(ctx context.Context) error {
						_, err := io.ReadAll(r)
						return err
					})
				}
				g.Wait()
			}
		})
	})
}
