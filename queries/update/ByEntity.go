package update

import (
	"testing"

	"github.com/adamluzsi/frameless/queries/queryerrors"
	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/fixtures"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// ByEntity will request an update for a wrapped entity object in the storage
// ByEntity parameter is the wrapped entity that has the updated values.
type ByEntity struct{ Entity frameless.Entity }

func (quc ByEntity) Test(suite *testing.T, storage frameless.Storage) {
	suite.Run("ByEntity", func(spec *testing.T) {

		setup := func() (string, func()) {
			entity := fixtures.New(quc.Entity)
			require.Nil(spec, storage.Store(entity))

			ID, ok := reflects.LookupID(entity)

			if !ok {
				spec.Fatal(queryerrors.ErrIDRequired)
			}

			require.True(spec, len(ID) > 0)
			return ID, func() { storage.Exec(destroy.ByEntity{Entity: quc.Entity}) }
		}

		spec.Run("values returned", func(t *testing.T) {
			ID, td := setup()
			defer td()

			newEntity := fixtures.New(quc.Entity)
			reflects.SetID(newEntity, ID)

			updateResults := storage.Exec(ByEntity{Entity: newEntity})
			require.NotNil(t, updateResults)
			require.Nil(t, updateResults.Err())

			iterator := storage.Exec(find.ByID{Type: quc.Entity, ID: ID})

			actually := fixtures.New(quc.Entity)
			iterators.DecodeNext(iterator, actually)

			require.Equal(t, newEntity, actually)

		})

		spec.Run("values in the storage but the requested entity that should be updated is not exists", func(t *testing.T) {
			_, td := setup()
			defer td()

			newEntity := fixtures.New(quc.Entity)
			reflects.SetID(newEntity, "hitchhiker's guide to the galaxy")
			require.Error(t, storage.Exec(ByEntity{Entity: newEntity}).Err())

		})

	})
}
