package inmemory_test

import (
	inmemory2 "github.com/adamluzsi/frameless/inmemory"
)

func ExampleStorage() {
	inmemory2.NewStorage(Entity{})
}
