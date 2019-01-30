package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// FindAll can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAll struct{ Type frameless.Entity }

func (quc FindAll) Test(t *testing.T, r resources.Resource) {
	t.Run("when value stored in the database", func(t *testing.T) {

		var ids []string

		for i := 0; i < 10; i++ {

			entity := newFixture(quc.Type)
			require.Nil(t, r.Exec(Save{Entity: entity}).Err())

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			ids = append(ids, id)
		}

		defer func() {
			for _, id := range ids {
				require.Nil(t, r.Exec(DeleteByID{Type: quc.Type, ID: id}).Err())
			}
		}()

		i := r.Exec(FindAll{Type: quc.Type})
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

	})

	t.Run("when no value present in the database", func(t *testing.T) {
		i := r.Exec(FindAll{Type: quc.Type})
		count, err := iterators.Count(i)
		require.Nil(t, err)
		require.Equal(t, 0, count)
	})

}
