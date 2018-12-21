package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

// FindByID request a business entity from a storage that implement it's test.
// Type is an empty struct from the given business entity type, and ID is a string
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindByID struct {
	Type frameless.Entity
	ID   string
}

// Test requires to be executed in a context where storage populated already with values for a given type
// also the caller should do the teardown as well
func (quc FindByID) Test(spec *testing.T, r frameless.Resource) {
	spec.Run("dependency", func(t *testing.T) {
		Save{Entity: quc.Type}.Test(t, r)
	})

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixtures.New(quc.Type)
		require.Nil(spec, r.Exec(Save{entity}).Err())
		ID, ok := resources.LookupID(entity)

		if !ok {
			spec.Fatal(frameless.ErrIDRequired)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

	}

	defer func() {
		for _, id := range ids {
			require.Nil(spec, r.Exec(DeleteByID{Type: quc.Type, ID: id}).Err())
		}
	}()

	spec.Run("when no value stored that the query request", func(t *testing.T) {
		iterator := r.Exec(FindByID{Type: quc.Type, ID: "The Cake Is a Lie"})
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
				iterator := r.Exec(FindByID{Type: quc.Type, ID: ID})

				defer iterator.Close()

				if err := iterators.DecodeNext(iterator, &entity); err != nil {
					t.Fatal(err)
				}
			}()

			actualID, ok := resources.LookupID(entity)

			if !ok {
				t.Fatal("can't find ID in the returned value")
			}

			require.Equal(t, ID, actualID)

		}
	})

}
