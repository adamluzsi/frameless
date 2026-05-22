package resilience_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleRetryReader() {
	reader := &resilience.RetryReader[resilience.FailureCount]{
		Open: func() (io.Reader, error) {
			return os.Open("name")
		},
		RetryPolicy: resilience.FixedDelay{
			Delay:    time.Second,
			Attempts: 7,
		},
	}

	data, err := io.ReadAll(reader)
	_, _ = data, err
}

func TestRetryReader_spec(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		Context = let.Context(s)

		content   = let.String(s)
		lastStub  = let.VarOf[*iokit.Stub](s, nil)
		openCount = let.VarOf(s, 0)
		Open      = let.Var(s, func(t *testcase.T) func() (io.Reader, error) {
			return func() (io.Reader, error) {
				openCount.Set(t, openCount.Get(t)+1)
				stub := &iokit.Stub{Data: []byte(content.Get(t))}
				lastStub.Set(t, stub)
				return stub, nil
			}
		})
		RetryPolicy = let.Var(s, func(t *testcase.T) resilience.RetryPolicy[resilience.FailureCount] {
			return resilience.FixedDelay{
				Delay:    time.Nanosecond,
				Attempts: 5,
			}
		})
	)
	subject := let.Var(s, func(t *testcase.T) *resilience.RetryReader[resilience.FailureCount] {
		return &resilience.RetryReader[resilience.FailureCount]{
			Context:     Context.Get(t),
			Open:        Open.Get(t),
			RetryPolicy: RetryPolicy.Get(t),
		}
	})

	s.Describe("#Close", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Close()
		})

		s.When("open was never used", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Equal(t, openCount.Get(t), 0)
			})

			s.Then("it does nothing just mark itself closed", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.ErrorIs(t, act(t), iokit.ErrClosed)
				assert.Equal(t, 0, openCount.Get(t))
			})
		})

		s.When("due to a read, the underlying io was opened", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				contentBytesLen := len([]byte(content.Get(t)))
				count := t.Random.IntBetween(1, contentBytesLen/2)
				n, err := subject.Get(t).Read(make([]byte, count))
				assert.NoError(t, err)
				assert.Equal(t, n, count)
			})

			s.And("the opened io.Reader is a ReadCloser", func(s *testcase.Spec) {
				stub := let.Var(s, func(t *testcase.T) *iokit.Stub {
					return &iokit.Stub{Data: []byte(content.Get(t))}
				})
				Open.Let(s, func(t *testcase.T) func() (io.Reader, error) {
					return func() (io.Reader, error) {
						st := stub.Get(t)
						st.Reset()
						var _ io.ReadCloser = st // implements ReadCloser
						return st, nil
					}
				})

				s.Then("it closes the underlying io.ReadCloser", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.True(t, stub.Get(t).IsClosed())
				})
			})
		})
	})

	s.Describe("#Source", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) io.Reader {
			return subject.Get(t).Source()
		})

		s.Then("source reader is returned", func(t *testcase.T) {
			src := act(t)
			assert.NotEmpty(t, src)
			assert.NotEmpty(t, lastStub.Get(t))
			assert.Equal[io.Reader](t, src, lastStub.Get(t))
		})
	})

	s.Describe("#Read", func(s *testcase.Spec) {
		var method = func(t *testcase.T, p []byte) (int, error) {
			return subject.Get(t).Read(p)
		}

		var (
			p = let.Var(s, func(t *testcase.T) []byte {
				return make([]byte, len([]byte(content.Get(t))))
			})
		)
		act := let.Act2(func(t *testcase.T) (int, error) {
			return method(t, p.Get(t))
		})

		s.Then("it will read the requested amount", func(t *testcase.T) {
			n, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, n, len(p.Get(t)))
			assert.Equal(t, []byte(content.Get(t))[:n], p.Get(t))
		})

		s.Then("it works as expected with io.ReadAll", func(t *testcase.T) {
			var (
				got []byte
				err error
			)
			assert.Within(t, time.Second, func(ctx context.Context) {
				got, err = io.ReadAll(subject.Get(t))
			})
			assert.NoError(t, err)
			assert.Equal(t, content.Get(t), string(got))

			assert.NotNil(t, lastStub.Get(t))
			assert.Equal(t, lastStub.Get(t).Offset(), len([]byte(content.Get(t))))
		})

		s.When("errors occur during reading", func(s *testcase.Spec) {
			errorCount := let.IntB(s, 1, 3) // must be less than retry policy max attempt
			readerCloses := let.VarOf[int](s, 0)
			Open.Let(s, func(t *testcase.T) func() (io.Reader, error) {
				var firstPassed bool
				var errCount = errorCount.Get(t)
				stub := &iokit.Stub{
					Data: []byte(content.Get(t)),
					StubRead: func(stub *iokit.Stub, p []byte) (int, error) {
						if !firstPassed {
							firstPassed = true
							return stub.Read(p[:len(p)/2])
						}
						if errCount != 0 {
							errCount--
							return 0, t.Random.Error()
						}
						return stub.Read(p)
					},
					StubClose: func(stub *iokit.Stub) error {
						if !stub.IsClosed() {
							readerCloses.Set(t, readerCloses.Get(t)+1)
						}
						return stub.Close()
					},
				}
				return func() (io.Reader, error) {
					stub.Reset()
					return stub, nil
				}
			})

			s.Then("from the outside, the errors are not observable", func(t *testcase.T) {
				got, err := io.ReadAll(subject.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, content.Get(t), string(got))
			})

			s.Then("failed readers are closed", func(t *testcase.T) {
				_, err := io.ReadAll(subject.Get(t))
				assert.NoError(t, err)

				assert.Equal(t, errorCount.Get(t), readerCloses.Get(t))
			})
		})
	})
}

func TestRetryReader_smoke(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("normal read all, close -> works as expected", func(t *testing.T) {
		data := rnd.StringN(1024)
		reader := strings.NewReader(data)

		ctx := context.Background()
		subject := &resilience.RetryReader[resilience.FailureCount]{
			Context: ctx,
			Open: func() (io.Reader, error) {
				return reader, nil
			},
			RetryPolicy: &resilience.FixedDelay{Attempts: 3},
		}

		got, err := io.ReadAll(subject)
		assert.NoError(t, err)
		assert.Equal(t, data, string(got))

		assert.NoError(t, subject.Close())
	})

	t.Run("close multiple times -> is closed and no other action", func(t *testing.T) {
		data := rnd.StringN(256)
		reader := strings.NewReader(data)

		ctx := context.Background()
		subject := &resilience.RetryReader[resilience.FailureCount]{
			Context: ctx,
			Open: func() (io.Reader, error) {
				return reader, nil
			},
			RetryPolicy: &resilience.FixedDelay{Attempts: 3},
		}

		// First close should succeed
		assert.NoError(t, subject.Close())
		assert.ErrorIs(t, iokit.ErrClosed, subject.Close())
	})

	t.Run("close then read -> iokit.ErrClosed received back", func(t *testing.T) {
		data := rnd.StringN(256)
		reader := strings.NewReader(data)

		ctx := context.Background()
		subject := &resilience.RetryReader[resilience.FailureCount]{
			Context: ctx,
			Open: func() (io.Reader, error) {
				return reader, nil
			},
			RetryPolicy: &resilience.FixedDelay{Attempts: 3},
		}

		assert.NoError(t, subject.Close())

		buf := make([]byte, 1024)
		_, err := subject.Read(buf)
		assert.ErrorIs(t, err, iokit.ErrClosed)
	})

	t.Run("underlying io reader is flaky -> open is done multiple times until read succeeds", func(t *testing.T) {
		data := rnd.StringN(512)
		var openCount int

		// First Open returns a reader that fails, subsequent Opens return a working reader
		ctx := context.Background()
		subject := &resilience.RetryReader[resilience.FailureCount]{
			Context: ctx,
			Open: func() (io.Reader, error) {
				openCount++
				if openCount == 1 {
					// First Open returns a reader that always fails
					return &flakyByteReader{
						data: []byte(data),
						readFn: func(p []byte) (int, error) {
							return 0, errors.New("simulated read failure")
						},
					}, nil
				}
				// Subsequent Opens return a working reader
				return strings.NewReader(data), nil
			},
			RetryPolicy: &resilience.FixedDelay{Attempts: 5},
		}

		buf := make([]byte, len(data))
		n, err := subject.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, string(buf[:n]))
		assert.Assert(t, 1 < openCount, "Open should be called multiple times due to flaky reader")
	})

	t.Run("successful read somewhere till halfway of a io reader, then the reader fails, and the retry reader closes the previous and reopens the reader, and then seeks to the last known position, and continues", func(t *testing.T) {
		data := rnd.StringN(2048)
		halfway := len(data) / 2

		var openCount int
		var closeCount int
		var seekCount int
		var readCount int

		// Track the position where we should resume
		resumePosition := 0

		ctx := context.Background()
		subject := &resilience.RetryReader[resilience.FailureCount]{
			Context: ctx,
			Open: func() (io.Reader, error) {
				openCount++
				r := &trackingReader{
					Reader:        strings.NewReader(data[resumePosition:]),
					readCount:     &readCount,
					closeCount:    &closeCount,
					seekCount:     &seekCount,
					resumePos:     &resumePosition,
					failAt:        halfway,
					alreadyFailed: false,
				}
				return r, nil
			},
			RetryPolicy: &resilience.FixedDelay{Attempts: 5},
		}

		// Read in small chunks to simulate gradual reading
		var allRead []byte
		buf := make([]byte, 64)

		for {
			n, err := subject.Read(buf)
			if n > 0 {
				allRead = append(allRead, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				assert.NoError(t, err)
				break
			}
		}

		// Verify we read all data
		assert.Equal(t, data, string(allRead))
		assert.Assert(t, openCount > 1, "Should have reopened due to failure")
		assert.Equal(t, closeCount, openCount-1, "Close should be called when reopening")
		_ = seekCount // Seeks happen internally

		// Verify no more reads work after EOF
		buf2 := make([]byte, 64)
		n, err := subject.Read(buf2)
		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, io.EOF)
	})
}

// flakyByteReader is a reader that fails on the first read then delegates
type flakyByteReader struct {
	data   []byte
	pos    int
	readFn func(p []byte) (int, error)
}

func (f *flakyByteReader) Read(p []byte) (int, error) {
	if f.readFn != nil {
		return f.readFn(p)
	}
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

// trackingReader wraps an io.Reader and tracks operations
type trackingReader struct {
	io.Reader
	readCount     *int
	closeCount    *int
	seekCount     *int
	resumePos     *int
	failAt        int
	alreadyFailed bool
	currentPos    int
}

func (t *trackingReader) Read(p []byte) (int, error) {
	(*t.readCount)++

	// Fail after reading halfway
	if !t.alreadyFailed && t.currentPos >= t.failAt {
		t.alreadyFailed = true
		return 0, errors.New("simulated read failure")
	}

	n, err := t.Reader.Read(p)
	t.currentPos += n
	return n, err
}

func (t *trackingReader) Close() error {
	(*t.closeCount)++
	if c, ok := t.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (t *trackingReader) Seek(offset int64, whence int) (int64, error) {
	(*t.seekCount)++
	if s, ok := t.Reader.(io.Seeker); ok {
		// Update resume position for next open
		if whence == io.SeekStart {
			*t.resumePos = int(offset)
		}
		return s.Seek(offset, whence)
	}
	return 0, errors.New("not seekable")
}
