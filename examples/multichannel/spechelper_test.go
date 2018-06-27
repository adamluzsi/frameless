package multichannel_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/examples/multichannel"
)

func ExampleUseCases(storage frameless.Storage) *multichannel.UseCases {
	return &multichannel.UseCases{Storage: storage}
}
