package memorystorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

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
	queries.Test(suite, memorystorage.NewMemory())
}
