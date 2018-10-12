package memorystorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

	"github.com/adamluzsi/frameless/externalresources/storages/memorystorage"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(suite *testing.T) {
	storage := ExampleMemory()

	queries.Test(suite, storage, storage.Purge)
}
