package crudcontracts_test

import (
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper"
)

func contracts[ENT, ID any](subject Subject[ENT, ID], cm comproto.OnePhaseCommitProtocol, opts ...crudcontracts.Option[ENT, ID]) []contract.Contract {
	return []contract.Contract{
		crudcontracts.Creator[ENT, ID](subject, opts...),
		crudcontracts.Finder[ENT, ID](subject, opts...),
		crudcontracts.Deleter[ENT, ID](subject, opts...),
		crudcontracts.Updater[ENT, ID](subject, opts...),
		crudcontracts.ByIDsFinder[ENT, ID](subject, opts...),
		crudcontracts.OnePhaseCommitProtocol[ENT, ID](subject, cm, opts...),
	}
}

type Subject[ENT, ID any] interface {
	crud.Creator[ENT]
	crud.Updater[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.ByIDsFinder[ENT, ID]
	crud.AllFinder[ENT]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
	spechelper.CRD[ENT, ID]
}
