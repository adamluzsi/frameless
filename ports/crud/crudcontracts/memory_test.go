package crudcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Test_memoryRepository(t *testing.T) {
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

	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID](subject, config),
		crudcontracts.Finder[Entity, ID](subject, config),
		crudcontracts.Deleter[Entity, ID](subject, config),
		crudcontracts.Updater[Entity, ID](subject, config),
		crudcontracts.OnePhaseCommitProtocol[Entity, ID](subject, m, config),
		crudcontracts.ByIDsFinder[Entity, ID](subject, config),
	)
}

func Test_cleanup(t *testing.T) {
	m := memory.NewEventLog()
	subject := memory.NewEventLogRepository[testent.Foo, testent.FooID](m)
	subject.Options.CompressEventLog = true

	crudConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	testcase.RunSuite(t,
		// crudcontracts.Creator[testent.Foo, testent.FooID](subject, crudConfig),
		// crudcontracts.Finder[testent.Foo, testent.FooID](subject, crudConfig),
		// crudcontracts.Updater[testent.Foo, testent.FooID](subject, crudConfig),
		// crudcontracts.Deleter[testent.Foo, testent.FooID](subject, crudConfig),
		crudcontracts.OnePhaseCommitProtocol[testent.Foo, testent.FooID](subject, subject.EventLog, crudConfig),
	)

	m.Compress()

	assert.Must(t).Empty(m.Events(),
		`after all the specs, the memory repository was expected to be empty.`+
			` If the repository has values, it means something is not cleaning up properly in the specs.`)
}
