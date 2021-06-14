package iterators_test

import (
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless"
)

type Entity struct {
	Text string
}

type ReadCloser struct {
	IsClosed bool
	io       io.Reader
}

func NewReadCloser(r io.Reader) *ReadCloser {
	return &ReadCloser{io: r, IsClosed: false}
}

func (this *ReadCloser) Read(p []byte) (n int, err error) {
	return this.io.Read(p)
}

func (this *ReadCloser) Close() error {
	if this.IsClosed {
		return errors.New("already closed")
	}

	this.IsClosed = true
	return nil
}

type BrokenReader struct{}

func (b *BrokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

type x struct{ data string }

type StubIterator struct {
	frameless.Decoder
	once   sync.Once
	closed bool
}

func (s *StubIterator) Close() error {
	s.closed = true
	return nil
}

func (s *StubIterator) Next() bool {
	var has bool
	s.once.Do(func() { has = true })
	return has
}

func (s *StubIterator) Err() error {
	return nil
}

func FirstAndLastSharedErrorTestCases(t *testing.T, subject func(iterators.Interface, interface{}) (bool, error)) {
	t.Run("error test-cases", func(t *testing.T) {
		expectedErr := errors.New(fixtures.Random.StringN(4))

		t.Run("Closing", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "close"}))

			i.StubClose = func() error { return expectedErr }

			_, err := subject(i, &Entity{})
			require.Equal(t, expectedErr, err)
		})

		t.Run("Decode", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "decode"}))

			i.StubDecode = func(interface{}) error { return expectedErr }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expectedErr, err)
		})

		t.Run("Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubErr = func() error { return expectedErr }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expectedErr, err)
		})

		t.Run("Decode+Close Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubDecode = func(interface{}) error { return expectedErr }
			i.StubClose = func() error { return errors.New("unexpected to see this err because it hides the decode err") }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expectedErr, err)
		})

		t.Run("Err+Close Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubErr = func() error { return expectedErr }
			i.StubClose = func() error { return errors.New("unexpected to see this err because it hides the decode err") }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expectedErr, err)
		})

		t.Run(`empty iterator with .Err()`, func(t *testing.T) {
			i := iterators.NewError(expectedErr)
			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expectedErr, err)
		})
	})
}
