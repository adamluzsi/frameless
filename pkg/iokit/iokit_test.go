package iokit_test

import (
	"bytes"
	"errors"
	"github.com/adamluzsi/testcase/assert"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/pkg/iokit"
	"github.com/adamluzsi/frameless/ports/filesystem"

	"github.com/adamluzsi/testcase"
)

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
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filesystem.ModeUserRWX)
			t.Must.Nil(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data.Get(t))
			t.Must.Nil(err)
			t.Must.Equal(len(data.Get(t)), n)
			t.Must.Nil(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			t.Must.Nil(err)
			t.Defer(f.Close)
			return f
		}}
	)

	thenContentsAreMatching := func(s *testcase.Spec) {
		s.Then("contents are matching with the reference ReadWriteSeeker", func(t *testcase.T) {
			expected, err := io.ReadAll(rwsc.Get(t))
			t.Must.Nil(err)
			actual, err := io.ReadAll(buffer.Get(t))
			t.Must.Nil(err)
			t.Must.Equal(string(expected), string(actual))
		})
	}

	thenContentsAreMatching(s)

	seekOnBoth := func(t *testcase.T, offset int64, whench int) {
		expected, err := rwsc.Get(t).Seek(offset, whench)
		t.Must.Nil(err)
		actual, err := buffer.Get(t).Seek(offset, whench)
		t.Must.Nil(err)
		t.Must.Equal(expected, actual, "seek length matches")
	}

	writeOnBoth := func(t *testcase.T, bs []byte) {
		expected, err := rwsc.Get(t).Write(bs)
		t.Must.Nil(err)
		actual, err := buffer.Get(t).Write(bs)
		t.Must.Nil(err)
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
			t.Must.Nil(buffer.Get(t).Close())
			t.Must.Nil(rwsc.Get(t).Close())
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
				t.Must.Nil(err)
				t.Must.Equal(string(expectedBS), buffer.Get(t).String())
				t.Must.Equal(expectedBS, buffer.Get(t).Bytes())
			})

			s.Then(".Bytes() matches with reference rwsc's read content", func(t *testcase.T) {
				rwsc.Get(t).Seek(0, io.SeekStart)
				expectedBS, err := io.ReadAll(rwsc.Get(t))
				t.Must.Nil(err)
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

func TestBuffer_smoke(tt *testing.T) {
	t := testcase.NewT(tt, nil)

	var (
		data   = []byte(t.Random.String())
		offset = int64(t.Random.IntN(1 + len(data)))
		whench = t.Random.ElementFromSlice([]int{io.SeekStart, io.SeekCurrent, io.SeekEnd}).(int)
		buffer = iokit.NewBuffer(data)
		tmpDir = t.TempDir()
		file   = func(t *testcase.T) filesystem.File {
			name := t.Random.StringNWithCharset(5, "qwerty")
			path := filepath.Join(tmpDir, name)
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filesystem.ModeUserRWX)
			t.Must.Nil(err)
			t.Defer(os.Remove, path)
			n, err := f.Write(data)
			t.Must.Nil(err)
			t.Must.Equal(len(data), n)
			t.Must.Nil(f.Close())

			f, err = os.OpenFile(path, os.O_RDWR, 0)
			t.Must.Nil(err)
			t.Defer(f.Close)
			return f
		}(t)
	)

	bseek, err := buffer.Seek(offset, whench)
	t.Must.Nil(err)

	fseek, err := file.Seek(offset, whench)
	t.Must.Nil(err)

	t.Must.Equal(fseek, bseek)

	wData := []byte(t.Random.StringN(1))
	bwrite, err := buffer.Write(wData)
	t.Must.Nil(err)
	fwrite, err := file.Write(wData)
	t.Must.Nil(err)
	t.Must.Equal(fwrite, bwrite)

	_, err = file.Seek(0, io.SeekStart)
	t.Must.Nil(err)
	_, err = buffer.Seek(0, io.SeekStart)
	t.Must.Nil(err)

	expectedBS, err := io.ReadAll(file)
	t.Must.Nil(err)
	actualBS, err := io.ReadAll(buffer)
	t.Must.Nil(err)
	t.Must.Equal(string(expectedBS), string(actualBS))
}

func TestNew_smoke(tt *testing.T) {
	t := testcase.NewT(tt, nil)
	dataSTR := t.Random.String()
	dataBS := []byte(t.Random.String())
	t.Must.Equal(iokit.NewBuffer(dataSTR).String(), dataSTR)
	t.Must.Equal(iokit.NewBuffer(dataBS).Bytes(), dataBS)
}

func TestBuffer_Read_ioReadAll(t *testing.T) {
	t.Run("on empty buffer", func(tt *testing.T) {
		t := testcase.NewT(tt, nil)
		b := &iokit.Buffer{}
		bs, err := io.ReadAll(b)
		t.Must.Nil(err)
		t.Must.Empty(bs)
	})
	t.Run("on populated buffer", func(tt *testing.T) {
		t := testcase.NewT(tt, nil)
		d := t.Random.String()
		b := iokit.NewBuffer(d)
		bs, err := io.ReadAll(b)
		t.Must.Nil(err)
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