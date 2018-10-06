package iterators_test

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOne(t *testing.T) {
	t.Run("describe", func(t *testing.T) {
		t.Run("when iterator have zero element", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewEmpty()
			var e frameless.Entity
			require.Equal(t, iterators.One(i, &e), iterators.ErrNoNextElement)
		})

		t.Run("when iterator have one element exactly", func(t *testing.T) {
			t.Parallel()

			var e string
			i := iterators.NewSingleElement("Hello, world!")
			require.Nil(t, iterators.One(i, &e))
			require.Equal(t, e, "Hello, world!")
		})

		t.Run("when iterator have more than one element", func(t *testing.T) {
			t.Parallel()

			var e int
			i := iterators.NewSlice([]int{1, 2, 3})
			require.Equal(t, iterators.One(i, &e), iterators.ErrUnexpectedNextElement)
		})

		t.Run("when iterator encounter an error during decode", func(t *testing.T) {
			t.Parallel()

			i := iterators.NewMock(iterators.NewEmpty())
			i.StubDecode = func(i interface{}) error {
				return fmt.Errorf("boom")
			}

			once := true
			i.StubNext = func() bool {
				if once {
					once = false
					return true
				}

				return false
			}

			var e int
			require.Equal(t, iterators.One(i, &e), fmt.Errorf("boom"))
		})

		t.Run("when iterator holds an error value", func(t *testing.T) {
			t.Run("before the iteration check", func(t *testing.T) {
				i := iterators.Errorf("boom")

				var e int
				require.Equal(t, iterators.One(i, &e), fmt.Errorf("boom"))
			})

			t.Run("after a *successful ran", func(t *testing.T) {
				i := iterators.NewMock(iterators.NewSingleElement("hello"))

				var once bool
				i.StubErr = func() error {
					if !once {
						once = true
						return nil
					}
					return fmt.Errorf("boom")
				}

				var e string
				require.Equal(t, iterators.One(i, &e), fmt.Errorf("boom"))
			})


			i := iterators.NewMock(iterators.NewEmpty())
			i.StubDecode = func(i interface{}) error {
				return fmt.Errorf("boom")
			}

			once := true
			i.StubNext = func() bool {
				if once {
					once = false
					return true
				}

				return false
			}

			var e int
			require.Equal(t, iterators.One(i, &e), fmt.Errorf("boom"))
		})
	})
}
