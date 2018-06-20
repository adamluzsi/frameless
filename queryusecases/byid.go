package queryusecases

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type ByID struct {
	Type frameless.Entity
	ID   string

	NewEntityForTest func(Type frameless.Entity) (NewUniqEntity frameless.Entity)
}

// Test requires to be executed in a context where storage populated already with values for a given type
// also the caller should do the teardown as well
func (quc ByID) Test(spec *testing.T, storage frameless.Storage) {

	if quc.NewEntityForTest == nil {
		spec.Fatal("without NewEntityForTest it this spec cannot work, but for usage outside of testing NewEntityForTest must not be used")
	}

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := quc.NewEntityForTest(quc.Type)
		require.Nil(spec, storage.Create(entity))
		ID, ok := reflects.LookupID(entity)

		if !ok {
			spec.Fatal(strings.Join([]string{
				"Can't find the ID in the current structure",
				"if there is no ID in the subject structure",
				"custom test needed that explicitly defines how ID is stored and retrived from an entity",
			}, "\n"))
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)
		// TODO: teardown
	}

	spec.Run("values returned", func(t *testing.T) {
		for _, ID := range ids {

			var entity frameless.Entity

			func() {
				iterator := storage.Find(ByID{Type: quc.Type, ID: ID})

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
