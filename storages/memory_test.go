package storages_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless/queryusecases"

	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"
)

type SampleEntity struct {
	ID   string
	Name string
}

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

			queryusecases.ByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("AllFor", func(t *testing.T) {
			t.Parallel()

			queryusecases.AllFor{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

	})

	suite.Run("Exec", func(spec *testing.T) {

		spec.Run("UpdateEntity", func(t *testing.T) {
			t.Parallel()

			queryusecases.UpdateEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("DeleteByID", func(t *testing.T) {
			t.Parallel()

			queryusecases.DeleteByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})

		spec.Run("DeleteByEntity", func(t *testing.T) {
			t.Parallel()

			queryusecases.DeleteByEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storages.NewMemory())
		})
	})
}
