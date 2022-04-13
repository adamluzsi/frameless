package contracts_test

import (
	"github.com/adamluzsi/frameless/cache/contracts"
	frmlscontracts "github.com/adamluzsi/frameless/contracts"
)

type ExampleEnt struct{ ID ExampleID }
type ExampleID string

var _ = []frmlscontracts.Interface{
	contracts.EntityStorage[ExampleEnt, ExampleID]{},
	contracts.Manager[ExampleEnt, ExampleID]{},
	contracts.Storage[ExampleEnt, ExampleID]{},
}
