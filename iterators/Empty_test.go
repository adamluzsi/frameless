package iterators_test

import (
	"math/rand"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func ExampleNewEmpty() iterators.Interface {
	return iterators.NewEmpty()
}

func TestNewEmpty(suite *testing.T) {
	suite.Run("#Close", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()

			subject := ExampleNewEmpty()

			require.Nil(t, subject.Close())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := ExampleNewEmpty()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				require.Nil(t, subject.Close())
			}
		})

	})

	suite.Run("#Next", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()

			subject := ExampleNewEmpty()

			require.False(t, subject.Next())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := ExampleNewEmpty()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				require.False(t, subject.Next())
			}
		})

	})

	suite.Run("#Err", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, ExampleNewEmpty().Err())
	})

	suite.Run("#Decode", func(t *testing.T) {
		t.Parallel()

		subject := ExampleNewEmpty()
		var entity interface{}

		require.Nil(t, subject.Decode(&entity))
		require.Nil(t, entity)
	})
}
