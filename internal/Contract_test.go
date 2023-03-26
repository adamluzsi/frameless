package internal

import (
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/metacontracts"
)

type (
	EntType   struct{ ID IDType }
	IDType    struct{}
	ValueType struct{}
)

var _ = []Contract{
	crudcontracts.Creator[EntType, IDType](nil),
	crudcontracts.Finder[EntType, IDType](nil),
	crudcontracts.QueryOne[EntType, IDType](nil),
	crudcontracts.Updater[EntType, IDType](nil),
	crudcontracts.Deleter[EntType, IDType](nil),
	crudcontracts.OnePhaseCommitProtocol[EntType, IDType](nil),
	frmetacontracts.MetaAccessor[ValueType](nil),
}
