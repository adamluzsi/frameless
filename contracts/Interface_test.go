package contracts

import (
	"github.com/adamluzsi/frameless/ports/comproto/contracts"
	"github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/meta/contracts"
	contracts2 "github.com/adamluzsi/frameless/ports/pubsub/contracts"
)

type (
	EntType   struct{ ID IDType }
	IDType    struct{}
	KeyType   struct{}
	ValueType struct{}
)

var _ = []Interface{
	crudcontracts.Creator[EntType, IDType]{},
	crudcontracts.Finder[EntType, IDType]{},
	crudcontracts.FindOne[EntType, IDType]{},
	crudcontracts.Updater[EntType, IDType]{},
	crudcontracts.Deleter[EntType, IDType]{},
	comprotocontracts.OnePhaseCommitProtocol[EntType, IDType]{},
	contracts2.Publisher[EntType, IDType]{},
	contracts2.CreatorPublisher[EntType, IDType]{},
	contracts2.UpdaterPublisher[EntType, IDType]{},
	contracts2.DeleterPublisher[EntType, IDType]{},
	frmetacontracts.MetaAccessor[EntType, KeyType, ValueType]{},
	frmetacontracts.MetaAccessorBasic[ValueType]{},
	frmetacontracts.MetaAccessorPublisher[EntType, KeyType, ValueType]{},
}
