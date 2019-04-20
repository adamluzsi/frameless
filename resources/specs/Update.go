package specs

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type Update interface {
	Update(ptr interface {}) error
}

// UpdateSpec will request an update for a wrapped entity object in the storage
// UpdateEntity parameter is the wrapped entity that has the updated values.
type UpdateSpec struct {
	Entity interface {}

	Subject interface {
		Update
		Save
		Delete
		FindByID
	}
}

func (quc UpdateSpec) Test(suite *testing.T) {
	suite.Run("Update", func(spec *testing.T) {

		setup := func(t *testing.T) (string, func()) {
			entity := newFixture(quc.Entity)
			require.Nil(spec, quc.Subject.Save(entity))

			ID, ok := LookupID(entity)

			if !ok {
				spec.Fatal(frameless.ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)

			td := func() { require.Nil(t, quc.Subject.Delete(entity)) }

			return ID, td
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Entity)
			SetID(newEntity, ID)

			err := quc.Subject.Update(newEntity)
			require.Nil(t, err)

			actually := newFixture(quc.Entity)
			ok, err := quc.Subject.FindByID(ID, actually)
			require.True(t, ok)
			require.Nil(t, err)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the r but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Entity)
			SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, quc.Subject.Update(newEntity))
		})

		spec.Run("given entity doesn't have an ID field", func(t *testing.T) {
			newEntity := newFixture(entityWithoutIDField{})

			require.Error(t, quc.Subject.Update(newEntity))
		})

	})
}
