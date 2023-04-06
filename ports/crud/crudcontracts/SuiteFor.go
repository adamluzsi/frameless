package crudcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/internal/suites"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"testing"
)

func SuiteFor[
	Entity, ID any,
	Resource suiteSubjectResource[Entity, ID],
](makeSubject func(testing.TB) SuiteSubject[Entity, ID, Resource]) Suite {
	T := any(*new(Resource))
	var contracts suites.Suites

	if _, ok := T.(crud.Purger); ok {
		contracts = append(contracts, Purger[Entity, ID](func(tb testing.TB) PurgerSubject[Entity, ID] {
			sub := makeSubject(tb)
			return PurgerSubject[Entity, ID]{
				Resource:    any(sub.Resource).(purgerSubjectResource[Entity, ID]),
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}))
	}

	contracts = append(contracts,
		Creator[Entity, ID](func(tb testing.TB) CreatorSubject[Entity, ID] {
			sub := makeSubject(tb)
			return CreatorSubject[Entity, ID]{
				Resource:        sub.Resource,
				MakeContext:     sub.MakeContext,
				MakeEntity:      sub.MakeEntity,
				SupportIDReuse:  sub.CreateSupportIDReuse,
				SupportRecreate: sub.CreateSupportRecreate,
			}
		}),
		ByIDFinder[Entity, ID](func(tb testing.TB) ByIDFinderSubject[Entity, ID] {
			sub := makeSubject(tb)
			return ByIDFinderSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		ByIDDeleter[Entity, ID](func(tb testing.TB) ByIDDeleterSubject[Entity, ID] {
			sub := makeSubject(tb)
			return ByIDDeleterSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		OnePhaseCommitProtocol[Entity, ID](func(tb testing.TB) OnePhaseCommitProtocolSubject[Entity, ID] {
			sub := makeSubject(tb)
			if sub.CommitManager == nil {
				tb.Skip("SuiteSubject.CommitManager is not supplied")
			}
			return OnePhaseCommitProtocolSubject[Entity, ID]{
				Resource:      sub.Resource,
				CommitManager: sub.CommitManager,
				MakeContext:   sub.MakeContext,
				MakeEntity:    sub.MakeEntity,
			}
		}),
	)

	if _, ok := T.(crud.Updater[Entity]); ok {
		contracts = append(contracts, Updater[Entity, ID](func(tb testing.TB) UpdaterSubject[Entity, ID] {
			sub := makeSubject(tb)
			return UpdaterSubject[Entity, ID]{
				Resource:    any(sub.Resource).(updaterSubjectResource[Entity, ID]),
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}))
	}

	if _, ok := T.(crud.AllFinder[Entity]); ok {
		contracts = append(contracts, AllFinder[Entity, ID](func(tb testing.TB) AllFinderSubject[Entity, ID] {
			sub := makeSubject(tb)
			return AllFinderSubject[Entity, ID]{
				Resource:    any(sub.Resource).(allFinderSubjectResource[Entity, ID]),
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}))
	}

	if _, ok := T.(crud.AllDeleter); ok {
		contracts = append(contracts, AllDeleter[Entity, ID](func(tb testing.TB) AllDeleterSubject[Entity, ID] {
			sub := makeSubject(tb)
			return AllDeleterSubject[Entity, ID]{
				Resource:    any(sub.Resource).(allDeleterSubjectResource[Entity, ID]),
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}))
	}

	return contracts
}

type SuiteSubject[
	Entity, ID any,
	Resource suiteSubjectResource[Entity, ID],
] struct {
	Resource              Resource
	CommitManager         comproto.OnePhaseCommitProtocol
	MakeContext           func() context.Context
	MakeEntity            func() Entity
	CreateSupportIDReuse  bool
	CreateSupportRecreate bool
}

type suiteSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}

type Suite suites.Suite
