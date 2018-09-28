package find

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/queries"
	"github.com/adamluzsi/frameless/queries/delete"
	"github.com/adamluzsi/frameless/queries/fixtures"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// All can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type All struct{ Type frameless.Entity }

func (quc All) Test(t *testing.T, storage frameless.Storage) {
	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixtures.New(quc.Type)
		require.Nil(t, storage.Store(entity))

		id, found := reflects.LookupID(entity)

		if !found {
			t.Fatal(queries.ErrIDRequired)
		}

		ids = append(ids, id)

		defer storage.Store(delete.ByID{Type: quc.Test, ID: id})

	}

	t.Run("Find", func(t *testing.T) {
		i := storage.Exec(quc)
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := reflects.LookupID(entity)

			if !found {
				t.Fatal(queries.ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

		require.Nil(t, i.Err())
	})

}
