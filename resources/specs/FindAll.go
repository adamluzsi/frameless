package specs

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"

	"github.com/stretchr/testify/require"
)

type FindAll interface {
	FindAll(Type interface{}) frameless.Iterator
}

type iFindAll interface {
	FindAll

	MinimumRequirements
}

// FindAllSpec can return business entities from a given storage that implement it's test
// The "Type" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAllSpec struct {
	Type interface{}

	Subject iFindAll
}

func (spec FindAllSpec) Test(t *testing.T) {
	t.Run("when value stored in the database", func(t *testing.T) {

		var ids []string

		for i := 0; i < 10; i++ {

			entity := newFixture(spec.Type)
			require.Nil(t, spec.Subject.Save(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			ids = append(ids, id)

			defer spec.Subject.DeleteByID(spec.Type, id)
		}

		i := spec.Subject.FindAll(spec.Type)
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(spec.Type)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

	})

	t.Run("when no value present in the database", func(t *testing.T) {
		i := spec.Subject.FindAll(spec.Type)
		count, err := iterators.Count(i)
		require.Nil(t, err)
		require.Equal(t, 0, count)
	})

}

func TestFindAll(t *testing.T, r iFindAll, e interface{}) {
	t.Run(`FindAll`, func(t *testing.T) {
		FindAllSpec{Type: e, Subject: r}.Test(t)
	})
}
