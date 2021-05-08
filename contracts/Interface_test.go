package contracts_test

import (
	contracts2 "github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
)

var _ = []testcase.Contract{
	contracts2.Creator{},
	contracts2.Finder{},
	contracts2.Updater{},
	contracts2.Deleter{},
	contracts2.OnePhaseCommitProtocol{},
	contracts2.CreatorPublisher{},
	contracts2.UpdaterPublisher{},
	contracts2.DeleterPublisher{},
}
