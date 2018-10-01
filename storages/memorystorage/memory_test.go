package memorystorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

	"github.com/adamluzsi/frameless/storages/memorystorage"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(suite *testing.T) {
	storage := ExampleMemory()

	reset := func() {
		for k, _ := range storage.DB {
			delete(storage.DB, k)
		}
	}

	queries.Test(suite, storage, reset)
}
