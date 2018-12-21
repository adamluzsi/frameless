package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"

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
func (q DeleteByID) Test(t *testing.T, r frameless.Resource) {

	t.Run("dependency", func(t *testing.T) {
		SaveEntity{Entity: q.Type}.Test(t, r)
	})

	t.Run("given database is populated", func(t *testing.T) {
		var ids []string

		for i := 0; i < 10; i++ {

			entity := fixtures.New(q.Type)
			require.Nil(t, r.Exec(SaveEntity{Entity: entity}).Err())
			ID, ok := resources.LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)
			ids = append(ids, ID)

		}

		t.Run("using delete by id makes entity with ID not find-able", func(t *testing.T) {
			for _, ID := range ids {

				i := r.Exec(FindByID{Type: q.Type, ID: ID})
				require.NotNil(t, i)
				require.Nil(t, i.Err())
				require.True(t, i.Next())
				require.Nil(t, i.Close())

				deleteResults := r.Exec(DeleteByID{Type: q.Type, ID: ID})
				require.NotNil(t, deleteResults)
				require.Nil(t, deleteResults.Err())

				i = r.Exec(FindByID{Type: q.Type, ID: ID})
				require.NotNil(t, i)
				require.Nil(t, i.Err())
				require.False(t, i.Next())
				require.Nil(t, i.Close())

			}
		})
	})

}
