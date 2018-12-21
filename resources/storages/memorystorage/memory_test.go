package memorystorage_test

import (
	"github.com/adamluzsi/frameless/errors"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
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
			require.Equal(t, errors.ErrNotImplemented, i.Err())
		})
	})

	t.Run("when subject query is manually implemented", func(t *testing.T) {

		storage.Implement(ImplementedQuery{}, func(m *memorystorage.Memory, q frameless.Query) frameless.Iterator {
			query := q.(ImplementedQuery)

			return iterators.NewSingleElement(query.Text)
		})

		t.Run("it is expected to execute the implementation function and return it's value", func(t *testing.T) {

			ExpectedValue, err := fixtures.RandomString(42)

			if err != nil {
				t.Fatal(err)
			}

			i := storage.Exec(ImplementedQuery{Text: ExpectedValue})

			var value string

			require.Nil(t, iterators.First(i, &value))
			require.Equal(t, ExpectedValue, value)
			require.Nil(t, i.Err())
		})

	})
}

type NotImplementedQuery struct{}

func (NotImplementedQuery) Test(t *testing.T, r frameless.Resource) {
	panic("not implemented")
}

type ImplementedQuery struct{ Text string }

func (ImplementedQuery) Test(t *testing.T, r frameless.Resource) {
	panic("not implemented")
}
