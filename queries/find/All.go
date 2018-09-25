package queries

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// AllFor can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type AllFor struct{ Type frameless.Entity }

func (quc AllFor) Test(t *testing.T, storage frameless.Storage, fixture frameless.Fixture) {
	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixture.New(quc.Type)
		require.Nil(t, storage.Store(entity))

		id, found := reflects.LookupID(entity)

		if !found {
			t.Fatal(idRequiredMessage)
		}

		ids = append(ids, id)

		defer storage.Store(DeleteByID{Type: quc.Test, ID: id})

	}

	t.Run("Find", func(t *testing.T) {
		i := storage.Find(quc)
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := reflects.LookupID(entity)

			if !found {
				t.Fatal(idRequiredMessage)
			}

			require.Contains(t, ids, id)
		}

		require.Nil(t, i.Err())
	})

}
