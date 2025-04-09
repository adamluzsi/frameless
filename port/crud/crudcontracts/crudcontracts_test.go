package crudcontracts_test

import (
	"strconv"
	"sync/atomic"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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
	crudcontracts.QueryOne[EntType, IDType](nil, "", nil),
	crudcontracts.QueryMany[EntType, IDType](nil, "", nil),
}

func contracts[ENT, ID any](resource Resource[ENT, ID], cm comproto.OnePhaseCommitProtocol, opts ...crudcontracts.Option[ENT, ID]) []contract.Contract {
	return []contract.Contract{
		crudcontracts.Creator[ENT, ID](resource, opts...),
		crudcontracts.Saver[ENT, ID](resource, opts...),
		crudcontracts.Finder[ENT, ID](resource, opts...),
		crudcontracts.Deleter[ENT, ID](resource, opts...),
		crudcontracts.Updater[ENT, ID](resource, opts...),
		crudcontracts.ByIDsFinder[ENT, ID](resource, opts...),
		crudcontracts.ByIDFinder[ENT, ID](resource, opts...),
		crudcontracts.AllFinder[ENT, ID](resource, opts...),
		crudcontracts.ByIDDeleter[ENT, ID](resource, opts...),
		crudcontracts.AllDeleter[ENT, ID](resource, opts...),
		crudcontracts.AllFinder[ENT, ID](resource, opts...),
		crudcontracts.OnePhaseCommitProtocol[ENT, ID](resource, cm, opts...),
		// crudcontracts.Purger[ENT, ID](resource, opts...),
	}
}

type Resource[ENT, ID any] interface {
	crud.Creator[ENT]
	crud.Saver[ENT]
	crud.Updater[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.ByIDsFinder[ENT, ID]
	crud.AllFinder[ENT]
	crud.ByIDDeleter[ID]
	crud.AllDeleter
	spechelper.CRD[ENT, ID]
	// crud.Purger
}

func Test_memory(t *testing.T) {
	type ID string
	type Entity struct {
		ID   ID `ext:"ID"`
		Data string
	}

	s := testcase.NewSpec(t)

	m := memory.NewMemory()
	resource := memory.NewRepository[Entity, ID](m)

	config := crudcontracts.Config[Entity, ID]{
		MakeEntity: func(tb testing.TB) Entity {
			return Entity{Data: testcase.ToT(&tb).Random.String()}
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	testcase.RunSuite(s, contracts[Entity, ID](resource, m, config)...)
}

func Test_fieldWithNoTaggedExtID(t *testing.T) {
	type DI string

	type Entity struct {
		DI   DI
		Data string
	}

	accessor := func(e *Entity) *DI {
		return &e.DI
	}

	s := testcase.NewSpec(t)

	m := memory.NewMemory()

	resource := &memory.Repository[Entity, DI]{
		Memory: m,
		IDA:    accessor,
	}

	config := crudcontracts.Config[Entity, DI]{
		MakeEntity: func(tb testing.TB) Entity {
			return Entity{Data: testcase.ToT(&tb).Random.String()}
		},
		SupportIDReuse:  true,
		SupportRecreate: true,

		IDA: accessor,
	}

	testcase.RunSuite(s, contracts[Entity, DI](resource, m, config)...)
}

func Test_memory_prepopulatedID(t *testing.T) {
	type ID string
	type Entity struct {
		ID   ID `ext:"ID"`
		Data string
	}

	s := testcase.NewSpec(t)

	m := memory.NewMemory()
	resource := memory.NewRepository[Entity, ID](m)
	resource.ExpectID = true

	var index int64
	config := crudcontracts.Config[Entity, ID]{
		MakeEntity: func(tb testing.TB) Entity {
			id := atomic.AddInt64(&index, 1)
			return Entity{
				ID:   ID(strconv.Itoa(int(id))),
				Data: testcase.ToT(&tb).Random.String(),
			}
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	testcase.RunSuite(s, contracts[Entity, ID](resource, m, config)...)
}

func Test_cleanup(t *testing.T) {
	m := memory.NewEventLog()
	resource := memory.NewEventLogRepository[testent.Foo, testent.FooID](m)
	resource.Options.CompressEventLog = true

	crudConfig := crudcontracts.Config[testent.Foo, testent.FooID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	}

	s := testcase.NewSpec(t)

	s.After(func(t *testcase.T) {
		// TODO: compress doesn't handle well if there is a case where previously a delete was made in a transaction for an entity, and then i was committed.
		// For some reason, it doesn't clean up the logs
		resource.Compress()
	})

	testcase.RunSuite(s, contracts[testent.Foo, testent.FooID](resource, m, crudConfig)...)
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

func Test_noleftoverAfterTests(t *testing.T) {
	mem := &memory.Memory{}
	resource := &memory.Repository[testent.Foo, testent.FooID]{Memory: mem}

	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		// TODO: something actually poops into the subject even before one of the test...
		spechelper.TryCleanup(t, t.Context(), resource)
	})

	s.After(func(t *testcase.T) {
		itr, err := resource.FindAll(t.Context())
		assert.NoError(t, err)

		vs, err := iterkit.CollectErr(itr)
		assert.NoError(t, err)

		t.OnFail(func() { t.LogPretty(vs) })

		assert.Empty(t, vs,
			`after all the specs, the memory repository was expected to be empty.`+
				` If the repository has values, it means something is not cleaning up properly in the specs.`)
	})

	testcase.RunSuite(s, contracts(resource, mem)...)
}

func Test_NoSkippedTestBecauseShouldStore(t *testing.T) {
	var check func(tb testing.TB, dtb *testcase.FakeTB)
	check = func(tb testing.TB, dtb *testcase.FakeTB) {
		msg := assert.Message(dtb.Logs.String())
		assert.False(tb, dtb.IsSkipped, msg)
		assert.False(tb, dtb.IsFailed, msg)
		for _, dtb := range dtb.Tests {
			check(tb, dtb)
		}
	}

	t.Run("with Creator", func(t *testing.T) {
		dtb := &testcase.FakeTB{}

		s := testcase.NewSpec(dtb)
		type Repo struct {
			crud.Creator[testent.Foo]
			crud.ByIDDeleter[testent.FooID]
			crud.ByIDFinder[testent.Foo, testent.FooID]
		}

		mrepo := &memory.Repository[testent.Foo, testent.FooID]{}
		var repo = Repo{
			Creator:     mrepo,
			ByIDDeleter: mrepo,
			ByIDFinder:  mrepo,
		}

		s.Context("smoke", crudcontracts.ByIDDeleter[testent.Foo, testent.FooID](repo).Spec)
		testcase.Sandbox(s.Finish)

		check(t, dtb)
	})

	t.Run("with Saver", func(t *testing.T) {
		dtb := &testcase.FakeTB{}

		s := testcase.NewSpec(dtb)
		type Repo struct {
			crud.Saver[testent.Foo]
			crud.ByIDDeleter[testent.FooID]
			crud.ByIDFinder[testent.Foo, testent.FooID]
		}

		mrepo := &memory.Repository[testent.Foo, testent.FooID]{}
		var repo = Repo{
			Saver:       mrepo,
			ByIDDeleter: mrepo,
			ByIDFinder:  mrepo,
		}

		s.Context("smoke", crudcontracts.ByIDDeleter[testent.Foo, testent.FooID](repo).Spec)
		testcase.Sandbox(s.Finish)

		check(t, dtb)
	})
}
