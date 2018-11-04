package queries

import (
	"github.com/adamluzsi/frameless/externalresources"
	"github.com/adamluzsi/frameless/queries/fixtures"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// FindAll can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAll struct{ Type frameless.Entity }

func (quc FindAll) Test(t *testing.T, storage frameless.Resource, reset func()) {
	t.Run("when value stored in the database", func(t *testing.T) {
		defer reset()

		ids := []string{}

		for i := 0; i < 10; i++ {

			entity := fixtures.New(quc.Type)
			require.Nil(t, storage.Exec(SaveEntity{Entity: entity}).Err())

			id, found := externalresources.LookupID(entity)

			if !found {
				t.Fatal(ErrIDRequired)
			}

			ids = append(ids, id)
		}

		i := storage.Exec(FindAll{Type: quc.Type})
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := externalresources.LookupID(entity)

			if !found {
				t.Fatal(ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

	})

	//t.Run("when no value present in the database", func(t *testing.T) {
	//	defer reset()
	//
	//	i := storage.Exec(FindAll{Type: quc.Type})
	//	count, err := iterateover.AndCountTotalIterations(i)
	//	require.Nil(t, err)
	//	require.Equal(t, 0, count)
	//})

}
