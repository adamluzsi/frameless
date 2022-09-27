package internal

import (
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/contracts"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
)

type (
	EntType   struct{ ID IDType }
	IDType    struct{}
	KeyType   struct{}
	ValueType struct{}
)

var _ = []Contract{
	crudcontracts.Creator[EntType, IDType]{},
	crudcontracts.Finder[EntType, IDType]{},
	crudcontracts.FindOne[EntType, IDType]{},
	crudcontracts.Updater[EntType, IDType]{},
	crudcontracts.Deleter[EntType, IDType]{},
	crudcontracts.OnePhaseCommitProtocol[EntType, IDType]{},
	pubsubcontracts.Publisher[EntType, IDType]{},
	pubsubcontracts.CreatorPublisher[EntType, IDType]{},
	pubsubcontracts.UpdaterPublisher[EntType, IDType]{},
	pubsubcontracts.DeleterPublisher[EntType, IDType]{},
	frmetacontracts.MetaAccessor[EntType, KeyType, ValueType]{},
	frmetacontracts.MetaAccessorBasic[ValueType]{},
	frmetacontracts.MetaAccessorPublisher[EntType, KeyType, ValueType]{},
}
