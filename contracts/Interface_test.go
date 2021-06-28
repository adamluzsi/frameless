package contracts_test

import (
	"github.com/adamluzsi/frameless/contracts"
)

var _ = []contracts.Interface{
	contracts.Creator{},
	contracts.Finder{},
	contracts.Updater{},
	contracts.Deleter{},
	contracts.OnePhaseCommitProtocol{},
	contracts.Publisher{},
	contracts.MetaAccessor{},
	contracts.MetaAccessorBasic{},
}
