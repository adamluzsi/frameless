package resourcespecs

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"

	"github.com/stretchr/testify/require"
)

type FindAll interface {
	FindAll(T frameless.Entity) frameless.Iterator
}

// FindAllSpec can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAllSpec struct {
	Type frameless.Entity

	Subject interface {
		FindAll
		Save
		Delete
	}
}

func (quc FindAllSpec) Test(t *testing.T) {
	t.Run("when value stored in the database", func(t *testing.T) {

		var ids []string

		for i := 0; i < 10; i++ {

			entity := newFixture(quc.Type)
			require.Nil(t, quc.Subject.Save(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			ids = append(ids, id)

			defer quc.Subject.Delete(entity)
		}

		i := quc.Subject.FindAll(quc.Type)
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
		i := quc.Subject.FindAll(quc.Type)
		count, err := iterators.Count(i)
		require.Nil(t, err)
		require.Equal(t, 0, count)
	})

}
