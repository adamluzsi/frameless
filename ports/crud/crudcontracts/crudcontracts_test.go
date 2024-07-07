package crudcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
)

type (
	EntType struct{ ID IDType }
	IDType  struct{}
)

var _ = []contract.Contract{
	crudcontracts.Creator[EntType, IDType](nil),
	crudcontracts.Finder[EntType, IDType](nil),
	crudcontracts.Updater[EntType, IDType](nil),
	crudcontracts.Deleter[EntType, IDType](nil),
	crudcontracts.OnePhaseCommitProtocol[EntType, IDType](nil, nil),
	crudcontracts.ByIDsFinder[EntType, IDType](nil),
	crudcontracts.AllFinder[EntType, IDType](nil),
	crudcontracts.QueryOne[EntType, IDType](nil, nil),
	crudcontracts.QueryMany[EntType, IDType](nil, nil, nil, nil),
}

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

func Test_memory(t *testing.T) {
	type ID string
	type Entity struct {
		ID   ID `ext:"ID"`
		Data string
	}

	s := testcase.NewSpec(t)

	m := memory.NewMemory()
	subject := memory.NewRepository[Entity, ID](m)

	config := crudcontracts.Config[Entity, ID]{
		MakeEntity: func(tb testing.TB) Entity {
			return Entity{Data: testcase.ToT(&tb).Random.String()}
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	testcase.RunSuite(s, contracts[Entity, ID](subject, m, config)...)
}

func Test_cleanup(t *testing.T) {
	m := memory.NewEventLog()
	subject := memory.NewEventLogRepository[testent.Foo, testent.FooID](m)
	subject.Options.CompressEventLog = true

	crudConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	s := testcase.NewSpec(t)

	s.After(func(t *testcase.T) {
		// TODO: compress doesn't handle well if there is a case where previously a delete was made in a transaction for an entity, and then i was committed.
		// For some reason, it doesn't clean up the logs
		subject.Compress()
	})

	testcase.RunSuite(s, contracts[testent.Foo, testent.FooID](subject, m, crudConfig)...)
}

func Test_preAssignedID(t *testing.T) {
	m := memory.NewEventLog()
	subject := memory.NewEventLogRepository[testent.Foo, testent.FooID](m)
	subject.Options.CompressEventLog = true

	crudConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		SupportIDReuse:  true,
		SupportRecreate: true,

		MakeEntity: func(tb testing.TB) testent.Foo {
			t := tb.(*testcase.T)
			return testent.Foo{
				ID:  testent.FooID(t.Random.UUID()),
				Foo: t.Random.StringN(5),
				Bar: t.Random.StringN(5),
				Baz: t.Random.StringN(5),
			}
		},
	}

	testcase.RunSuite(t, contracts[testent.Foo, testent.FooID](subject, m, crudConfig)...)
}
