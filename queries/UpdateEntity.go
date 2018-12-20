package queries

import (
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/adamluzsi/frameless"
)

// UpdateEntity will request an update for a wrapped entity object in the storage
// UpdateEntity parameter is the wrapped entity that has the updated values.
type UpdateEntity struct{ Entity frameless.Entity }

func (quc UpdateEntity) Test(suite *testing.T, r frameless.Resource) {
	suite.Run("UpdateEntity", func(spec *testing.T) {

		suite.Run("dependency", func(t *testing.T) {
			SaveEntity{Entity: quc.Entity}.Test(t, r)
		})

		setup := func(t *testing.T) (string, func()) {
			entity := fixtures.New(quc.Entity)
			require.Nil(spec, r.Exec(SaveEntity{Entity: entity}).Err())

			ID, ok := resources.LookupID(entity)

			if !ok {
				spec.Fatal(ErrIDRequired)
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

			newEntity := fixtures.New(quc.Entity)
			resources.SetID(newEntity, ID)

			updateResults := r.Exec(UpdateEntity{Entity: newEntity})
			require.NotNil(t, updateResults)
			require.Nil(t, updateResults.Err())

			iterator := r.Exec(FindByID{Type: quc.Entity, ID: ID})

			actually := fixtures.New(quc.Entity)
			iterators.DecodeNext(iterator, actually)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the r but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup(t)
			defer td()

			newEntity := fixtures.New(quc.Entity)
			resources.SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, r.Exec(UpdateEntity{Entity: newEntity}).Err())

		})

		spec.Run("given entity doesn't have an ID field", func(t *testing.T) {
			newEntity := fixtures.New(entityWithoutIDField{})
			require.Error(t, r.Exec(UpdateEntity{Entity: newEntity}).Err())
		})

	})
}
