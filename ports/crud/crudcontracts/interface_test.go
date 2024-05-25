package crudcontracts

import "go.llib.dev/frameless/ports/contract"

type (
	EntType struct{ ID IDType }
	IDType  struct{}
)

var _ = []contract.Contract{
	Creator[EntType, IDType](nil),
	Finder[EntType, IDType](nil),
	Updater[EntType, IDType](nil),
	Deleter[EntType, IDType](nil),
	OnePhaseCommitProtocol[EntType, IDType](nil, nil),
	ByIDsFinder[EntType, IDType](nil),
	AllFinder[EntType, IDType](nil),

	QueryOne[EntType, IDType](nil, nil),
	QueryMany[EntType, IDType](nil, nil, nil, nil),
}
