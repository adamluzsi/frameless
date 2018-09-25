package delete

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

// ByEntity request a delete of a specific entity that is wrapped in the query use case object
type ByEntity struct {
	Entity frameless.Entity
}

// Test will test that an ByEntity is implemented by a generic specification
func (quc ByEntity) Test(spec *testing.T, storage frameless.Storage, fixture frameless.Fixture) {

	if fixture == nil {
		spec.Fatal("without NewEntityForTest it this spec cannot work, but for usage outside of testing NewEntityForTest must not be used")
	}

	expected := fixture.New(quc.Entity)
	require.Nil(spec, storage.Store(expected))
	ID, ok := reflects.LookupID(expected)

	if !ok {
		spec.Fatal(idRequiredMessage)
	}

	spec.Run("value is Deleted by providing an Entity, and than it should not be findable afterwards", func(t *testing.T) {

		require.Nil(t, storage.Exec(ByEntity{Entity: expected}))

		iterator := storage.Find(ByID{Type: quc.Entity, ID: ID})
		defer iterator.Close()

		if iterator.Next() {
			var entity frameless.Entity
			iterator.Decode(&entity)
			t.Fatalf("there should be no next value, but %#v found", entity)
		}

	})

}
