package iterators_test

import (
	"errors"
	"github.com/adamluzsi/frameless/pkg/iterators"
	"io"
	"testing"

	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
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

func FirstAndLastSharedErrorTestCases[T any](t *testing.T, subject func(iterators.Iterator[Entity]) (T, bool, error)) {
	t.Run("error test-cases", func(t *testing.T) {
		expectedErr := errors.New(random.New(random.CryptoSeed{}).StringN(4))

		t.Run("Closing", func(t *testing.T) {
			t.Parallel()

			expected := Entity{Text: "close"}
			i := iterators.SingleValue[Entity](expected)

			v, ok, err := subject(i)
			assert.Must(t).Nil(err)
			assert.Must(t).True(ok)
			assert.Must(t).Equal(expected, v)
		})

		t.Run("Closing", func(t *testing.T) {
			t.Parallel()

			i := iterators.Stub[Entity](iterators.SingleValue[Entity](Entity{Text: "close"}))

			i.StubClose = func() error { return expectedErr }

			_, _, err := subject(i)
			assert.Must(t).Equal(expectedErr, err)
		})

		t.Run("Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.Stub[Entity](iterators.SingleValue[Entity](Entity{Text: "err"}))
			i.StubErr = func() error { return expectedErr }

			_, _, err := subject(i)
			assert.Must(t).Equal(expectedErr, err)
		})

		t.Run("Err+Close Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.Stub[Entity](iterators.SingleValue[Entity](Entity{Text: "err"}))
			i.StubErr = func() error { return expectedErr }
			i.StubClose = func() error { return errors.New("unexpected to see this err because it hides the decode err") }

			_, _, err := subject(i)
			assert.Must(t).Equal(expectedErr, err)
		})

		t.Run(`empty iterator with .Err()`, func(t *testing.T) {
			i := iterators.Error[Entity](expectedErr)
			_, found, err := subject(i)
			assert.Must(t).Equal(false, found)
			assert.Must(t).Equal(expectedErr, err)
		})
	})
}
