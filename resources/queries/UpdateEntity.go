package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
	"testing"
)

// UpdateEntity will request an update for a wrapped entity object in the storage
// UpdateEntity parameter is the wrapped entity that has the updated values.
type UpdateEntity struct{ Entity frameless.Entity }

func (quc UpdateEntity) Test(suite *testing.T, r resources.Resource) {
	suite.Run("UpdateEntity", func(spec *testing.T) {

		suite.Run("dependency", func(t *testing.T) {
			Save{Entity: quc.Entity}.Test(t, r)
		})

		setup := func(t *testing.T) (string, func()) {
			entity := newFixture(quc.Entity)
			require.Nil(spec, r.Exec(Save{Entity: entity}).Err())

			ID, ok := LookupID(entity)

			if !ok {
				spec.Fatal(frameless.ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)

			td := func() {
				require.Nil(t, r.Exec(DeleteByID{Type: reflects.BaseValueOf(quc.Entity).Interface(), ID: ID}).Err())
			}

			return ID, td
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Entity)
			SetID(newEntity, ID)

			updateResults := r.Exec(UpdateEntity{Entity: newEntity})
			require.NotNil(t, updateResults)
			require.Nil(t, updateResults.Err())

			iterator := r.Exec(FindByID{Type: quc.Entity, ID: ID})

			actually := newFixture(quc.Entity)
			iterators.DecodeNext(iterator, actually)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the r but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup(t)
			defer td()

			newEntity := newFixture(quc.Entity)
			SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, r.Exec(UpdateEntity{Entity: newEntity}).Err())

		})

		spec.Run("given entity doesn't have an ID field", func(t *testing.T) {
			newEntity := newFixture(entityWithoutIDField{})
			require.Error(t, r.Exec(UpdateEntity{Entity: newEntity}).Err())
		})

	})
}
