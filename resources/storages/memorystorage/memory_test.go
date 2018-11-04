package memorystorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

	"github.com/adamluzsi/frameless/resources/storages/memorystorage"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(suite *testing.T) {
	storage := ExampleMemory()

	queries.TestAll(suite, storage, storage.Purge)
}
