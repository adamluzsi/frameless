package destroy

import (
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/save"
	"github.com/adamluzsi/frameless/storages"
	"testing"

	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/adamluzsi/frameless/queries/queryerrors"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// ByEntity request a destroy of a specific entity that is wrapped in the query use case object
type ByEntity struct {
	Entity frameless.Entity
}

// Test will test that an ByEntity is implemented by a generic specification
func (quc ByEntity) Test(spec *testing.T, storage frameless.Storage, reset func()) {
	defer reset()

	spec.Run("dependency", func(t *testing.T) {
		save.Entity{Entity: quc.Entity}.Test(t, storage, reset)
	})

	expected := fixtures.New(quc.Entity)
	require.Nil(spec, storage.Exec(save.Entity{Entity: expected}).Err())
	ID, ok := storages.LookupID(expected)

	if !ok {
		spec.Fatal(queryerrors.ErrIDRequired)
	}

	spec.Run("value is Deleted by providing an Entity, and than it should not be findable afterwards", func(t *testing.T) {

		deleteResults := storage.Exec(ByEntity{Entity: expected})
		require.NotNil(t, deleteResults)
		require.Nil(t, deleteResults.Err())

		// TODO: fix it to use BaseValueOf Entity
		iterator := storage.Exec(find.ByID{Type: quc.Entity, ID: ID})
		defer iterator.Close()

		if iterator.Next() {
			var entity frameless.Entity
			iterator.Decode(&entity)
			t.Fatalf("there should be no next value, but %#v found", entity)
		}

	})

	spec.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		defer reset()

		newEntity := fixtures.New(entityWithoutIDField{})
		require.Error(t, storage.Exec(ByEntity{Entity: newEntity}).Err())
	})
}

type entityWithoutIDField struct {
	Data string
}
