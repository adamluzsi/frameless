package usecases_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/example/usecases"
)

func ExampleUseCases(storage frameless.Storage) *usecases.UseCases {
	return usecases.NewUseCases(storage)
}
