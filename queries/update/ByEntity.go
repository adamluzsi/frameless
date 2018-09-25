package queries

import (
	"testing"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// UpdateEntity will request an update for a wrapped entity object in the storage
// Entity parameter is the wrapped entity that has the updated values.
type UpdateEntity struct{ Entity frameless.Entity }

func (quc UpdateEntity) Test(suite *testing.T, storage frameless.Storage, fixture frameless.Fixture) {

	if fixture.New == nil {
		suite.Fatal("without NewEntityForTest it this spec cannot work, but for usage outside of testing NewEntityForTest must not be used")
	}

	suite.Run("UpdateEntity", func(spec *testing.T) {

		setup := func() (string, func()) {
			entity := fixture.New(quc.Entity)
			require.Nil(spec, storage.Store(entity))

			ID, ok := reflects.LookupID(entity)

			if !ok {
				spec.Fatal(ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)
			return ID, func() { storage.Exec(DeleteByEntity{Entity: quc.Entity}) }
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup()
			defer td()

			newEntity := fixture.New(quc.Entity)
			reflects.SetID(newEntity, ID)

			require.Nil(t, storage.Exec(UpdateEntity{Entity: newEntity}))

			iterator := storage.Exec(ByID{Type: quc.Entity, ID: ID})

			actually := fixture.New(quc.Entity)
			iterators.DecodeNext(iterator, actually)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the storage but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup()
			defer td()

			newEntity := fixture.New(quc.Entity)
			reflects.SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, storage.Exec(UpdateEntity{Entity: newEntity}).Err())

		})

	})
}
