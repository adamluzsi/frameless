package queries

import (
	"github.com/adamluzsi/frameless/externalresources"
	"testing"

	"github.com/adamluzsi/frameless/queries/fixtures"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

// DeleteByID request to destroy a business entity in the storage that implement it's test.
// Type is an empty struct from the given business entity type, and ID is a string
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type DeleteByID struct {
	Type frameless.Entity
	ID   string
}

// Test will test that an DeleteByID is implemented by a generic specification
func (quc DeleteByID) Test(spec *testing.T, storage frameless.Resource, reset func()) {
	defer reset()

	spec.Run("dependency", func(t *testing.T) {
		SaveEntity{Entity: quc.Type}.Test(t, storage, reset)
	})

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixtures.New(quc.Type)
		require.Nil(spec, storage.Exec(SaveEntity{Entity: entity}).Err())
		ID, ok := externalresources.LookupID(entity)

		if !ok {
			spec.Fatal(ErrIDRequired)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

	}

	spec.Run("value is Deleted after exec", func(t *testing.T) {
		for _, ID := range ids {

			deleteResults := storage.Exec(DeleteByID{Type: quc.Type, ID: ID})
			require.NotNil(t, deleteResults)
			require.Nil(t, deleteResults.Err())

			iterator := storage.Exec(DeleteByID{Type: quc.Type, ID: ID})
			defer iterator.Close()

			var entity frameless.Entity
			require.Equal(t, iterators.ErrNoNextElement, iterators.DecodeNext(iterator, &entity))

		}
	})

}
