package iterators_test

import (
	"math/rand"
	"testing"

	"go.llib.dev/frameless/ports/iterators"

	"github.com/adamluzsi/testcase/assert"
)

func ExampleEmpty() {
	iterators.Empty[any]()
}

func TestEmpty(suite *testing.T) {
	suite.Run("#Close", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()
			subject := iterators.Empty[any]()
			assert.Must(t).Nil(subject.Close())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := iterators.Empty[any]()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				assert.Must(t).Nil(subject.Close())
			}
		})

	})

	suite.Run("#Next", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()

			subject := iterators.Empty[any]()

			assert.Must(t).False(subject.Next())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := iterators.Empty[any]()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				assert.Must(t).False(subject.Next())
			}
		})

	})

	suite.Run("#Err", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Nil(iterators.Empty[any]().Err())
	})

	suite.Run("#Value", func(t *testing.T) {
		t.Parallel()
		subject := iterators.Empty[int]()
		assert.Must(t).Equal(0, subject.Value())
	})
}
