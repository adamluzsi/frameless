package memorystorage_test

import (
	"github.com/adamluzsi/frameless/resources/queries"
	"github.com/adamluzsi/frameless/resources/storages"
	"github.com/adamluzsi/frameless/resources/storages/memorystorage"

	"testing"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(t *testing.T) {
	storage := ExampleMemory()
	queries.TestAll(t, storage)
	storages.TestCommonSpec(t, storage)
}
