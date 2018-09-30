package find

import (
	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/adamluzsi/frameless/queries/queryerrors"
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// All can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type All struct{ Type frameless.Entity }

func (quc All) Test(t *testing.T, storage frameless.Storage, reset func()) {
	t.Run("when value stored in the database", func(t *testing.T) {
		defer reset()

		ids := []string{}

		for i := 0; i < 10; i++ {

			entity := fixtures.New(quc.Type)
			require.Nil(t, storage.Store(entity))

			id, found := reflects.LookupID(entity)

			if !found {
				t.Fatal(queryerrors.ErrIDRequired)
			}

			ids = append(ids, id)
		}

		i := storage.Exec(All{Type: quc.Type})
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := reflects.LookupID(entity)

			if !found {
				t.Fatal(queryerrors.ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

	})

	//t.Run("when no value present in the database", func(t *testing.T) {
	//	defer reset()
	//
	//	i := storage.Exec(All{Type: quc.Type})
	//	count, err := iterateover.AndCountTotalIterations(i)
	//	require.Nil(t, err)
	//	require.Equal(t, 0, count)
	//})

}
