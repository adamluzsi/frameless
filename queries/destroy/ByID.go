package destroy

import (
	"github.com/adamluzsi/frameless/queries/persist"
	"github.com/adamluzsi/frameless/storages"
	"testing"

	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/adamluzsi/frameless/queries/queryerrors"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

// ByID request to destroy a business entity in the storage that implement it's test.
// Type is an empty struct from the given business entity type, and ID is a string
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type ByID struct {
	Type frameless.Entity
	ID   string
}

// Test will test that an ByID is implemented by a generic specification
func (quc ByID) Test(spec *testing.T, storage frameless.Storage, reset func()) {
	defer reset()

	spec.Run("dependency", func(t *testing.T) {
		persist.Entity{Entity: quc.Type}.Test(t, storage, reset)
	})

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixtures.New(quc.Type)
		require.Nil(spec, storage.Exec(persist.Entity{Entity: entity}).Err())
		ID, ok := storages.LookupID(entity)

		if !ok {
			spec.Fatal(queryerrors.ErrIDRequired)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

	}

	spec.Run("value is Deleted after exec", func(t *testing.T) {
		for _, ID := range ids {

			deleteResults := storage.Exec(ByID{Type: quc.Type, ID: ID})
			require.NotNil(t, deleteResults)
			require.Nil(t, deleteResults.Err())

			iterator := storage.Exec(ByID{Type: quc.Type, ID: ID})
			defer iterator.Close()

			var entity frameless.Entity
			require.Equal(t, iterators.ErrNoNextElement, iterators.DecodeNext(iterator, &entity))

		}
	})

}
