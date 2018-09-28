package find

import (
	"github.com/adamluzsi/frameless/queries"
	"github.com/adamluzsi/frameless/queries/delete"
	"github.com/adamluzsi/frameless/queries/fixtures"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// ByID request a business entity from a storage that implement it's test.
// Type is an empty struct from the given business entity type, and ID is a string
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type ByID struct {
	Type frameless.Entity
	ID   string
}

// Test requires to be executed in a context where storage populated already with values for a given type
// also the caller should do the teardown as well
func (quc ByID) Test(spec *testing.T, storage frameless.Storage) {

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixtures.New(quc.Type)
		require.Nil(spec, storage.Store(entity))
		ID, ok := reflects.LookupID(entity)

		if !ok {
			spec.Fatal(queries.ErrIDRequired)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

		defer storage.Exec(delete.ByID{Type: quc.Test, ID: ID})
	}

	spec.Run("when no value stored that the query request", func(t *testing.T) {
		iterator := storage.Exec(ByID{Type: quc.Type, ID: "The Cake Is a Lie"})
		defer iterator.Close()

		if iterator.Next() {
			var entity frameless.Entity
			iterator.Decode(&entity)
			t.Fatalf("there should be no next value, but %#v found", entity)
		}
	})

	spec.Run("values returned", func(t *testing.T) {
		for _, ID := range ids {

			var entity frameless.Entity

			func() {
				iterator := storage.Exec(ByID{Type: quc.Type, ID: ID})

				defer iterator.Close()

				if err := iterators.DecodeNext(iterator, &entity); err != nil {
					t.Fatal(err)
				}
			}()

			actualID, ok := reflects.LookupID(entity)

			if !ok {
				t.Fatal("can't find ID in the returned value")
			}

			require.Equal(t, ID, actualID)

		}
	})

}
