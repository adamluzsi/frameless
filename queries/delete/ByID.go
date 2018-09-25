package delete

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

// DeleteByID request to delete a business entity in the storage that implement it's test.
// Type is an empty struct from the given business entity type, and ID is a string
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type DeleteByID struct {
	Type frameless.Entity
	ID   string

}

// Test will test that an DeleteByID is implemented by a generic specification
func (quc DeleteByID) Test(spec *testing.T, storage frameless.Storage, fixture frameless.Fixture) {

	if fixture == nil {
		spec.Fatal("without NewEntityForTest it this spec cannot work, but for usage outside of testing NewEntityForTest must not be used")
	}

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := fixture.New(quc.Type)
		require.Nil(spec, storage.Store(entity))
		ID, ok := reflects.LookupID(entity)

		if !ok {
			spec.Fatal(idRequiredMessage)
		}

		require.True(spec, len(ID) > 0)
		ids = append(ids, ID)

	}

	spec.Run("value is Deleted after exec", func(t *testing.T) {
		for _, ID := range ids {

			require.Nil(t, storage.Exec(DeleteByID{Type: quc.Type, ID: ID}))

			iterator := storage.Find(ByID{Type: quc.Type, ID: ID})
			defer iterator.Close()

			var entity frameless.Entity
			require.Equal(t, iterators.ErrNoNextElement, iterators.DecodeNext(iterator, &entity))

		}
	})

}

// DeleteByEntity request a delete of a specific entity that is wrapped in the query use case object
type DeleteByEntity struct {
	Entity frameless.Entity
}

// Test will test that an DeleteByEntity is implemented by a generic specification
func (quc DeleteByEntity) Test(spec *testing.T, storage frameless.Storage, fixture frameless.Fixture) {

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

		require.Nil(t, storage.Exec(DeleteByEntity{Entity: expected}))

		iterator := storage.Find(ByID{Type: quc.Entity, ID: ID})
		defer iterator.Close()

		if iterator.Next() {
			var entity frameless.Entity
			iterator.Decode(&entity)
			t.Fatalf("there should be no next value, but %#v found", entity)
		}

	})

}
