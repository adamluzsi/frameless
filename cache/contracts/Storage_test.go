package contracts_test

import (
	"github.com/adamluzsi/frameless/cache/contracts"
	frmlscontracts "github.com/adamluzsi/frameless/contracts"
)

var _ = []frmlscontracts.Interface{
	contracts.EntityStorage{},
	contracts.Manager{},
	contracts.Storage{},
}
