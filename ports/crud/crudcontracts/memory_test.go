package crudcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
)

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
