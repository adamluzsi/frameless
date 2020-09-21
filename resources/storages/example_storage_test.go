package storages_test

import (
	"github.com/adamluzsi/frameless/resources/storages"
)

func ExampleStorage() {
	storages.NewMemory()
}
