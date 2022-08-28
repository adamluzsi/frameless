package cachecontracts_test

import (
	frmlscontracts "github.com/adamluzsi/frameless/contracts"
	cachecontracts "github.com/adamluzsi/frameless/ports/crud/cache/contracts"
)

type ExampleEnt struct{ ID ExampleID }
type ExampleID string

var _ = []frmlscontracts.Interface{
	cachecontracts.EntityStorage[ExampleEnt, ExampleID]{},
	cachecontracts.Manager[ExampleEnt, ExampleID]{},
	cachecontracts.Storage[ExampleEnt, ExampleID]{},
}
