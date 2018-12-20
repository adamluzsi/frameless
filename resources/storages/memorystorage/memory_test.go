package memorystorage_test

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources/storages/memorystorage"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(suite *testing.T) {
	storage := ExampleMemory()

	queries.TestAll(suite, storage)
}

func TestImplement(t *testing.T) {
	storage := ExampleMemory()

	t.Run("when subject query is not being implemented manually", func(t *testing.T) {
		t.Run("it is expected to return not implemented query error", func(t *testing.T) {
			storage := ExampleMemory()
			i := storage.Exec(NotImplementedQuery{})
			require.Equal(t, queries.ErrNotImplemented, i.Err())
		})
	})

	t.Run("when subject query is manually implemented", func(t *testing.T) {

		storage.Implement(ImplementedQuery{}, func(*memorystorage.Memory) frameless.Iterator {
			return iterators.NewSingleElement("Works!")
		})

		t.Run("it is expected to execute the implementation function and return it's value", func(t *testing.T) {
			i := storage.Exec(ImplementedQuery{})

			var value string

			require.Nil(t, iterators.First(i, &value))
			require.Equal(t, "Works!", value)
			require.Nil(t, i.Err())
		})

	})
}

type NotImplementedQuery struct{}

func (NotImplementedQuery) Test(t *testing.T, r frameless.Resource) {
	panic("not implemented")
}

type ImplementedQuery struct{}

func (ImplementedQuery) Test(t *testing.T, r frameless.Resource) {
	panic("not implemented")
}
