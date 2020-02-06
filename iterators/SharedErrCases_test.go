package iterators_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func SharedErrCases(t *testing.T, subject func(iterators.Interface, interface{}) (bool, error)) {
	t.Run("ErrCases", func(t *testing.T) {
		expected := errors.New("error")

		t.Run("Closing", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "close"}))

			i.StubClose = func() error { return expected }

			_, err := subject(i, &Entity{})
			require.Equal(t, expected, err)
		})

		t.Run("Decode", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "decode"}))

			i.StubDecode = func(interface{}) error { return expected }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expected, err)
		})

		t.Run("Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubErr = func() error { return expected }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expected, err)
		})

		t.Run("Decode+Close Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubDecode = func(interface{}) error { return expected }
			i.StubClose = func() error { return errors.New("unexpected to see this err because it hides the decode err") }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expected, err)
		})

		t.Run("Err+Close Err", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewSingleElement(Entity{Text: "err"}))

			i.StubErr = func() error { return expected }
			i.StubClose = func() error { return errors.New("unexpected to see this err because it hides the decode err") }

			found, err := subject(i, &Entity{})
			require.Equal(t, false, found)
			require.Equal(t, expected, err)
		})

	})
}
