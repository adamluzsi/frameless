package cachecontracts_test

import (
	frmlscontracts "go.llib.dev/frameless/internal/suites"
	cachecontracts "go.llib.dev/frameless/pkg/cache/cachecontracts"
)

type ExampleEnt struct{ ID ExampleID }
type ExampleID string

var _ = []frmlscontracts.Suite{
	cachecontracts.EntityRepository[ExampleEnt, ExampleID](nil),
	cachecontracts.Cache[ExampleEnt, ExampleID](nil),
	cachecontracts.Repository[ExampleEnt, ExampleID](nil),
}
