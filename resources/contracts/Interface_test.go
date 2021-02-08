package contracts_test

import (
	"github.com/adamluzsi/frameless/resources/contracts"
	"github.com/adamluzsi/testcase"
)

var _ = []testcase.Contract{
	contracts.Creator{},
	contracts.Finder{},
	contracts.Updater{},
	contracts.Deleter{},
	contracts.OnePhaseCommitProtocol{},
	contracts.CreatorPublisher{},
	contracts.UpdaterPublisher{},
	contracts.DeleterPublisher{},
}
