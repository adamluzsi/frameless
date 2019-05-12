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

// FindAllSpec can return business entities from a given storage that implement it's test
// The "EntityType" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type FindAllSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject iFindAll
}

type iFindAll interface {
	FindAll

	MinimumRequirements
}

func (spec FindAllSpec) Test(t *testing.T) {
	t.Run("when value stored in the database", func(t *testing.T) {

		var ids []string

		for i := 0; i < 10; i++ {

			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Save(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			ids = append(ids, id)

			defer spec.Subject.DeleteByID(spec.EntityType, id)
		}

		i := spec.Subject.FindAll(spec.EntityType)
		defer i.Close()

		for i.Next() {
			entity := reflect.New(reflect.TypeOf(spec.EntityType)).Interface()

			require.Nil(t, i.Decode(entity))

			id, found := LookupID(entity)

			if !found {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.Contains(t, ids, id)
		}

	})

	t.Run("when no value present in the database", func(t *testing.T) {
		i := spec.Subject.FindAll(spec.EntityType)
		count, err := iterators.Count(i)
		require.Nil(t, err)
		require.Equal(t, 0, count)
	})

}

func TestFindAll(t *testing.T, r iFindAll, e interface{}, f FixtureFactory) {
	t.Run(`FindAll`, func(t *testing.T) {
		FindAllSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})
}
