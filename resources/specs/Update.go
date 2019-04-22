package specs

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type Update interface {
	Update(ptr interface{}) error
}

type iUpdate interface {
	Update

	MinimumRequirements
}

// UpdateSpec will request an update for a wrapped entity object in the resource
type UpdateSpec struct {
	Type interface{}

	Subject iUpdate
}

func (quc UpdateSpec) Test(suite *testing.T) {
	suite.Run("Update", func(spec *testing.T) {

		setup := func(t *testing.T) (string, func()) {
			entity := newFixture(quc.Type)
			require.Nil(spec, quc.Subject.Save(entity))

			ID, ok := LookupID(entity)

			if !ok {
				spec.Fatal(frameless.ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)

			td := func() { require.Nil(t, quc.Subject.DeleteByID(quc.Type, ID)) }

			return ID, td
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Type)
			require.Nil(t, SetID(newEntity, ID))

			err := quc.Subject.Update(newEntity)
			require.Nil(t, err)

			actually := newFixture(quc.Type)
			ok, err := quc.Subject.FindByID(ID, actually)
			require.True(t, ok)
			require.Nil(t, err)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the r but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Type)
			require.Nil(t, SetID(newEntity, "hitchhiker's guide to the galaxy"))
			require.Error(t, quc.Subject.Update(newEntity))
		})

		spec.Run("given entity doesn't have an ID field", func(t *testing.T) {
			newEntity := newFixture(entityWithoutIDField{})

			require.Error(t, quc.Subject.Update(newEntity))
		})

	})
}

func TestUpdate(t *testing.T, r iUpdate, e interface{}) {
	t.Run(`Update`, func(t *testing.T) {
		UpdateSpec{Type: e, Subject: r}.Test(t)
	})
}
