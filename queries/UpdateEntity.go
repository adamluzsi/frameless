package queries

import (
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/adamluzsi/frameless"
)

// UpdateEntity will request an update for a wrapped entity object in the storage
// UpdateEntity parameter is the wrapped entity that has the updated values.
type UpdateEntity struct{ Entity frameless.Entity }

func (quc UpdateEntity) Test(suite *testing.T, storage frameless.Resource, reset func()) {
	suite.Run("UpdateEntity", func(spec *testing.T) {

		suite.Run("dependency", func(t *testing.T) {
			SaveEntity{Entity: quc.Entity}.Test(t, storage, reset)
		})

		setup := func() (string, func()) {
			entity := fixtures.New(quc.Entity)
			require.Nil(spec, storage.Exec(SaveEntity{Entity: entity}).Err())

			ID, ok := resources.LookupID(entity)

			if !ok {
				spec.Fatal(ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)

			return ID, reset
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup()
			defer td()

			newEntity := fixtures.New(quc.Entity)
			resources.SetID(newEntity, ID)

			updateResults := storage.Exec(UpdateEntity{Entity: newEntity})
			require.NotNil(t, updateResults)
			require.Nil(t, updateResults.Err())

			iterator := storage.Exec(FindByID{Type: quc.Entity, ID: ID})

			actually := fixtures.New(quc.Entity)
			iterators.DecodeNext(iterator, actually)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the storage but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup()
			defer td()

			newEntity := fixtures.New(quc.Entity)
			resources.SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, storage.Exec(UpdateEntity{Entity: newEntity}).Err())

		})

		spec.Run("given entity doesn't have storage ID field", func(t *testing.T) {
			defer reset()

			newEntity := fixtures.New(entityWithoutIDField{})
			require.Error(t, storage.Exec(UpdateEntity{Entity: newEntity}).Err())
		})

	})
}
