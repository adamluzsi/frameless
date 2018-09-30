package memorystorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"github.com/adamluzsi/frameless/storages"
	"testing"

	"github.com/adamluzsi/frameless/storages/memorystorage"

	"github.com/stretchr/testify/require"
)

func TestMemoryStore_SpecificValueGiven_IDSet(t *testing.T) {
	t.Parallel()

	storage := memorystorage.NewMemory()
	entity := NewEntityForTest(SampleEntity{})

	require.Nil(t, storage.Store(entity))

	ID, ok := storages.LookupID(entity)

	require.True(t, ok, "ID is not defined in the entity struct src definition")
	require.True(t, len(ID) > 0)
}

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(suite *testing.T) {
	storage := ExampleMemory()

	queries.Test(suite, storage, func() { *storage = *ExampleMemory() })
}
