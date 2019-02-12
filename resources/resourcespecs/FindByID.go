package resourcespecs

import (
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

type FindByID interface {
	FindByID(ID string, ptr frameless.Entity) (bool, error)
}

type FindByIDSpec struct {
	Type frameless.Entity

	Subject interface {
		FindByID
		Save
		DeleteByID
	}
}

func (quc FindByIDSpec) Test(spec *testing.T) {

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := newFixture(quc.Type)

		require.Nil(spec, quc.Subject.Save(entity))
		ID, ok := LookupID(entity)

		if !ok {
			spec.Fatal(frameless.ErrIDRequired)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

	}

	defer func() {
		for _, id := range ids {
			require.Nil(spec, quc.Subject.DeleteByID(quc.Type, id))
		}
	}()

	spec.Run("when no value stored that the query request", func(t *testing.T) {
		var entity frameless.Entity
		ok, err := quc.Subject.FindByID("not existing ID", &entity)

		require.Nil(t, err)
		require.False(t, ok)
	})

	spec.Run("values returned", func(t *testing.T) {
		for _, ID := range ids {

			var entity frameless.Entity

			ok, err := quc.Subject.FindByID(ID, &entity)
			require.Nil(t, err)
			require.True(t, ok)

			actualID, ok := LookupID(entity)

			if !ok {
				t.Fatal("can't find ID in the returned value")
			}

			require.Equal(t, ID, actualID)

		}
	})

}
