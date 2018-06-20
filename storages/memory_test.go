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

func TestMemoryFind(suite *testing.T) {
	suite.Run("QueryUseCases", func(spec *testing.T) {

		storage := storages.NewMemory()

		spec.Run("ByID", func(t *testing.T) {
			t.Parallel()

			queryusecases.ByID{
				Type:             SampleEntity{},
				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})

	})
}
