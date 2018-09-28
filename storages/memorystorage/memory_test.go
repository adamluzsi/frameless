package memorystorage_test

import (
	"testing"

	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/update"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/storages/memorystorage"

	"github.com/stretchr/testify/require"
)

func TestMemoryStore_SpecificValueGiven_IDSet(t *testing.T) {
	t.Parallel()

	storage := memorystorage.NewMemory()
	entity := NewEntityForTest(SampleEntity{})

	require.Nil(t, storage.Store(entity))

	ID, ok := reflects.LookupID(entity)

	require.True(t, ok, "ID is not defined in the entity struct src definition")
	require.True(t, len(ID) > 0)
}

func TestMemory(suite *testing.T) {
	suite.Run("Find", func(spec *testing.T) {

		spec.Run("ByID", func(t *testing.T) {
			t.Parallel()

			find.ByID{Type: SampleEntity{}}.Test(t, memorystorage.NewMemory())
		})

		spec.Run("FindAll", func(t *testing.T) {
			t.Parallel()

			find.All{Type: SampleEntity{}}.Test(t, memorystorage.NewMemory())
		})

	})

	suite.Run("Exec", func(spec *testing.T) {

		spec.Run("UpdateEntity", func(t *testing.T) {
			t.Parallel()

			update.ByEntity{Entity: SampleEntity{}}.Test(t, memorystorage.NewMemory())
		})

		spec.Run("DeleteByID", func(t *testing.T) {
			t.Parallel()

			destroy.ByID{Type: SampleEntity{}}.Test(t, memorystorage.NewMemory())
		})

		spec.Run("DeleteByEntity", func(t *testing.T) {
			t.Parallel()

			destroy.ByEntity{Entity: SampleEntity{}}.Test(t, memorystorage.NewMemory())
		})
	})
}
