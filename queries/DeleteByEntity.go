package queries

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/adamluzsi/frameless/queries/fixtures"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// DeleteByEntity request a destroy of a specific entity that is wrapped in the query use case object
type DeleteByEntity struct {
	Entity frameless.Entity
}

// Test will test that an DeleteByEntity is implemented by a generic specification
func (quc DeleteByEntity) Test(spec *testing.T, storage frameless.Resource, reset func()) {
	defer reset()

	spec.Run("dependency", func(t *testing.T) {
		SaveEntity{Entity: quc.Entity}.Test(t, storage, reset)
	})

	expected := fixtures.New(quc.Entity)
	require.Nil(spec, storage.Exec(SaveEntity{Entity: expected}).Err())
	ID, ok := resources.LookupID(expected)

	if !ok {
		spec.Fatal(ErrIDRequired)
	}

	spec.Run("value is Deleted by providing an SaveEntity, and than it should not be findable afterwards", func(t *testing.T) {

		deleteResults := storage.Exec(DeleteByEntity{Entity: expected})
		require.NotNil(t, deleteResults)
		require.Nil(t, deleteResults.Err())

		// TODO: fix it to use BaseValueOf SaveEntity
		iterator := storage.Exec(FindByID{Type: quc.Entity, ID: ID})
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
		require.Error(t, storage.Exec(DeleteByEntity{Entity: newEntity}).Err())
	})
}
