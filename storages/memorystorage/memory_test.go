package storages_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless/queries"

	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"
)

func TestMemoryCreate_SpecificValueGiven_IDSet(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	entity := NewEntityForTest(SampleEntity{})

	require.Nil(t, storage.Create(entity))

	ID, ok := reflects.LookupID(entity)

	require.True(t, ok, "ID is not defined in the entity struct src definition")
	require.True(t, len(ID) > 0)
}

func TestMemory(suite *testing.T) {
	suite.Run("Find", func(spec *testing.T) {

		spec.Run("ByID", func(t *testing.T) {
			t.Parallel()

			queries.ByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("AllFor", func(t *testing.T) {
			t.Parallel()

			queries.AllFor{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

	})

	suite.Run("Exec", func(spec *testing.T) {

		spec.Run("UpdateEntity", func(t *testing.T) {
			t.Parallel()

			queries.UpdateEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("DeleteByID", func(t *testing.T) {
			t.Parallel()

			queries.DeleteByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("DeleteByEntity", func(t *testing.T) {
			t.Parallel()

			queries.DeleteByEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})
	})
}
