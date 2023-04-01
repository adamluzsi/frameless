package cachecontracts_test

import (
	frmlscontracts "github.com/adamluzsi/frameless/internal"
	cachecontracts "github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
)

type ExampleEnt struct{ ID ExampleID }
type ExampleID string

var _ = []frmlscontracts.Contract{
	cachecontracts.EntityRepository[ExampleEnt, ExampleID](nil),
	cachecontracts.Cache[ExampleEnt, ExampleID](nil),
	cachecontracts.Repository[ExampleEnt, ExampleID](nil),
}