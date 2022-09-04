package cachecontracts_test

import (
	frmlscontracts "github.com/adamluzsi/frameless/internal"
	cachecontracts "github.com/adamluzsi/frameless/ports/crud/cache/contracts"
)

type ExampleEnt struct{ ID ExampleID }
type ExampleID string

var _ = []frmlscontracts.Contract{
	cachecontracts.EntityStorage[ExampleEnt, ExampleID]{},
	cachecontracts.Manager[ExampleEnt, ExampleID]{},
	cachecontracts.Storage[ExampleEnt, ExampleID]{},
}
