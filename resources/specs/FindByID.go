package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

type FindByID interface {
	FindByID(ID string, PTR interface {}) (bool, error)
}

type FindByIDSpec struct {
	Type interface {}

	Subject interface {
		FindByID
		Save
		DeleteByID
	}
}

func (spec FindByIDSpec) Test(t *testing.T) {

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := newFixture(spec.Type)

		require.Nil(t, spec.Subject.Save(entity))
		ID, ok := LookupID(entity)

		if !ok {
			t.Fatal(frameless.ErrIDRequired)
		}

		require.True(t, len(ID) > 0)
		ids = append(ids, ID)

	}

	defer func() {
		for _, id := range ids {
			require.Nil(t, spec.Subject.DeleteByID(spec.Type, id))
		}
	}()

	t.Run("when no value stored that the query request", func(t *testing.T) {
		ptr := reflects.New(spec.Type)
		ok, err := spec.Subject.FindByID("not existing ID", ptr)

		require.Nil(t, err)
		require.False(t, ok)
	})

	t.Run("values returned", func(t *testing.T) {
		for _, ID := range ids {

			entityPtr := reflects.New(spec.Type)
			ok, err := spec.Subject.FindByID(ID, entityPtr)

			require.Nil(t, err)
			require.True(t, ok)

			actualID, ok := LookupID(entityPtr)

			if !ok {
				t.Fatal("can't find ID in the returned value")
			}

			require.Equal(t, ID, actualID)

		}
	})

}
