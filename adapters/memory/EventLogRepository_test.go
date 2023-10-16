package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/ports/meta/metacontracts"
	"go.llib.dev/frameless/spechelper/resource"
	"go.llib.dev/frameless/spechelper/testent"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

var _ interface {
	crud.Creator[TestEntity]
	crud.Finder[TestEntity, string]
	crud.Updater[TestEntity]
	crud.Deleter[string]
	comproto.OnePhaseCommitProtocol
} = &memory.EventLogRepository[TestEntity, string]{}

var _ cache.EntityRepository[TestEntity, string] = &memory.EventLogRepository[TestEntity, string]{}

func TestEventLogRepository(t *testing.T) {
	testcase.RunSuite(t, resource.Contract[TestEntity, string](func(tb testing.TB) resource.ContractSubject[TestEntity, string] {
		m := memory.NewEventLog()
		s := memory.NewEventLogRepository[TestEntity, string](m)
		return resource.ContractSubject[TestEntity, string]{
			Resource:      s,
			MetaAccessor:  m,
			CommitManager: m,
			MakeContext:   context.Background,
			MakeEntity:    func() TestEntity { return makeTestEntity(tb) },
		}
	}))
	repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
	contracts := getRepositorySpecs[TestEntity](repository, makeTestEntity)
	testcase.RunSuite(t, contracts...)
}

func TestEventLogRepository_smoke(t *testing.T) {
	var (
		subject = memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		ctx     = context.Background()
		count   int
		err     error
	)

	assert.Must(t).Nil(subject.Create(ctx, &TestEntity{Data: `A`}))
	assert.Must(t).Nil(subject.Create(ctx, &TestEntity{Data: `B`}))
	count, err = iterators.Count(subject.FindAll(ctx))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(2, count)

	assert.Must(t).Nil(subject.DeleteAll(ctx))
	count, err = iterators.Count(subject.FindAll(ctx))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(0, count)

	tx1CTX, err := subject.BeginTx(ctx)
	assert.Must(t).Nil(err)
	assert.Must(t).Nil(subject.Create(tx1CTX, &TestEntity{Data: `C`}))
	count, err = iterators.Count(subject.FindAll(tx1CTX))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(1, count)
	assert.Must(t).Nil(subject.RollbackTx(tx1CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(0, count)

	tx2CTX, err := subject.BeginTx(ctx)
	assert.Must(t).Nil(err)
	assert.Must(t).Nil(subject.Create(tx2CTX, &TestEntity{Data: `D`}))
	count, err = iterators.Count(subject.FindAll(tx2CTX))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(1, count)
	assert.Must(t).Nil(subject.CommitTx(tx2CTX))
	count, err = iterators.Count(subject.FindAll(ctx))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(1, count)

	assert.Must(t).Nil(subject.DeleteAll(ctx))
	count, err = iterators.Count(subject.FindAll(ctx))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(0, count)
}

func getRepositorySpecsForT[Entity, ID any](
	subject *memory.EventLogRepository[Entity, ID],
	MakeContext func(testing.TB) context.Context,
	MakeEntity func(testing.TB) Entity,
) []testcase.Suite {
	return []testcase.Suite{
		crudcontracts.Creator[Entity, ID](func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
			return crudcontracts.CreatorSubject[Entity, ID]{
				Resource:        subject,
				MakeContext:     context.Background,
				MakeEntity:      func() Entity { return MakeEntity(tb) },
				SupportIDReuse:  true,
				SupportRecreate: false,
			}
		}),
		crudcontracts.Finder[Entity, ID](func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
			return crudcontracts.FinderSubject[Entity, ID]{
				Resource:    subject,
				MakeContext: func() context.Context { return MakeContext(tb) },
				MakeEntity:  func() Entity { return MakeEntity(tb) },
			}
		}),
		crudcontracts.Updater[Entity, ID](func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
			return crudcontracts.UpdaterSubject[Entity, ID]{
				Resource:    subject,
				MakeContext: func() context.Context { return MakeContext(tb) },
				MakeEntity:  func() Entity { return MakeEntity(tb) },
			}
		}),
		crudcontracts.Deleter[Entity, ID](func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
			return crudcontracts.DeleterSubject[Entity, ID]{
				Resource:    subject,
				MakeContext: func() context.Context { return MakeContext(tb) },
				MakeEntity:  func() Entity { return MakeEntity(tb) },
			}
		}),
		crudcontracts.OnePhaseCommitProtocol[Entity, ID](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
			return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
				Resource:      subject,
				MakeContext:   func() context.Context { return MakeContext(tb) },
				MakeEntity:    func() Entity { return MakeEntity(tb) },
				CommitManager: subject.EventLog,
			}
		}),
		cachecontracts.EntityRepository[Entity, ID](func(tb testing.TB) cachecontracts.EntityRepositorySubject[Entity, ID] {
			return cachecontracts.EntityRepositorySubject[Entity, ID]{
				EntityRepository: subject,
				MakeContext:      func() context.Context { return MakeContext(tb) },
				MakeEntity:       func() Entity { return MakeEntity(tb) },
				CommitManager:    subject.EventLog,
			}
		}),
		metacontracts.MetaAccessor[int](func(tb testing.TB) metacontracts.MetaAccessorSubject[int] {
			return metacontracts.MetaAccessorSubject[int]{
				MetaAccessor: subject.EventLog,
				MakeContext:  context.Background,
				MakeV:        testcase.ToT(&tb).Random.Int,
			}
		}),
	}
}

func getRepositorySpecs[Entity, ID any](
	subject *memory.EventLogRepository[Entity, ID],
	MakeEntity func(testing.TB) Entity,
) []testcase.Suite {
	makeContext := func(testing.TB) context.Context { return context.Background() }
	return getRepositorySpecsForT[Entity, ID](subject, makeContext, MakeEntity)
}

func TestEventLogRepository_multipleInstanceTransactionOnTheSameContext(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run(`with create in different comproto`, func(t *testing.T) {
		subject1 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		subject2 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())

		ctx := context.Background()
		ctx, err := subject1.BeginTx(ctx)
		assert.Must(t).Nil(err)
		ctx, err = subject2.BeginTx(ctx)
		assert.Must(t).Nil(err)

		t.Log(`when in subject 1 store an entity`)
		entity := &TestEntity{Data: `42`}
		assert.Must(t).Nil(subject1.Create(ctx, entity))

		t.Log(`and subject 2 finish comproto`)
		assert.Must(t).Nil(subject2.CommitTx(ctx))
		t.Log(`and subject 2 then try to find this entity`)
		_, found, err := subject2.FindByID(context.Background(), entity.ID)
		assert.Must(t).Nil(err)
		assert.Must(t).False(found, `it should not see the uncommitted entity`)

		t.Log(`but after subject 1 commit the comproto`)
		assert.Must(t).Nil(subject1.CommitTx(ctx))
		t.Log(`subject 1 can see the newT entity`)
		_, found, err = subject1.FindByID(context.Background(), entity.ID)
		assert.Must(t).Nil(err)
		assert.Must(t).True(found)
	})

	t.Run(`deletes across comproto instances in the same context`, func(t *testing.T) {
		subject1 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		subject2 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())

		ctx := context.Background()
		e1 := rnd.Make(TestEntity{}).(TestEntity)
		e1.ID = ""
		e2 := rnd.Make(TestEntity{}).(TestEntity)
		e2.ID = ""

		assert.Must(t).Nil(subject1.Create(ctx, &e1))
		id1, ok := extid.Lookup[string](e1)
		assert.Must(t).True(ok)
		assert.Must(t).NotEmpty(id1)
		t.Cleanup(func() { _ = subject1.DeleteByID(context.Background(), id1) })

		assert.Must(t).Nil(subject2.Create(ctx, &e2))
		id2, ok := extid.Lookup[string](e2)
		assert.Must(t).True(ok)
		assert.Must(t).NotEmpty(id2)
		t.Cleanup(func() { _ = subject2.DeleteByID(context.Background(), id2) })

		ctx, err := subject1.BeginTx(ctx)
		assert.Must(t).Nil(err)
		ctx, err = subject2.BeginTx(ctx)
		assert.Must(t).Nil(err)

		_, found, err := subject1.FindByID(ctx, id1)
		assert.Must(t).Nil(err)
		assert.Must(t).True(found)
		assert.Must(t).Nil(subject1.DeleteByID(ctx, id1))

		_, found, err = subject2.FindByID(ctx, id2)
		assert.Must(t).True(found)
		assert.Must(t).Nil(subject2.DeleteByID(ctx, id2))

		_, found, err = subject1.FindByID(ctx, id1)
		assert.Must(t).Nil(err)
		assert.Must(t).False(found)

		_, found, err = subject2.FindByID(ctx, id2)
		assert.Must(t).Nil(err)
		assert.Must(t).False(found)

		_, found, err = subject1.FindByID(context.Background(), id1)
		assert.Must(t).Nil(err)
		assert.Must(t).True(found)

		assert.Must(t).Nil(subject1.CommitTx(ctx))
		assert.Must(t).Nil(subject2.CommitTx(ctx))

		_, found, err = subject1.FindByID(context.Background(), id1)
		assert.Must(t).Nil(err)
		assert.Must(t).False(found)

	})
}

func TestEventLogRepository_Options_CompressEventLog(t *testing.T) {
	m := memory.NewEventLog()
	subject := memory.NewEventLogRepository[TestEntity, string](m)
	subject.Options.CompressEventLog = true

	testcase.RunSuite(t, getRepositorySpecs[TestEntity, string](subject, makeTestEntity)...)

	for _, event := range m.Events() {
		t.Logf("namespace:%s -> event:%#v", subject.GetNamespace(), event)
	}

	assert.Must(t).Empty(m.Events(),
		`after all the specs, the memory repository was expected to be empty.`+
			` If the repository has values, it means something is not cleaning up properly in the specs.`)
}

func TestEventLogRepository_NewIDFunc(t *testing.T) {
	t.Run(`when NewID is absent`, func(t *testing.T) {
		repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		repository.MakeID = nil

		ptr := &TestEntity{Data: "42"}
		assert.Must(t).Nil(repository.Create(context.Background(), ptr))
		assert.Must(t).NotEmpty(ptr.ID)
	})

	t.Run(`when NewID is provided`, func(t *testing.T) {
		repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		expectedID := random.New(random.CryptoSeed{}).String()
		repository.MakeID = func(ctx context.Context) (string, error) {
			return expectedID, nil
		}

		ptr := &TestEntity{Data: "42"}
		assert.Must(t).Nil(repository.Create(context.Background(), ptr))
		assert.Must(t).Equal(expectedID, ptr.ID)
	})
}

func TestEventLogRepository_CompressEvents_smoke(t *testing.T) {
	el := memory.NewEventLog()

	type A struct {
		ID string `ext:"ID"`
		V  string
	}
	type B struct {
		ID string `ext:"ID"`
		V  string
	}

	ctx := context.Background()
	aS := memory.NewEventLogRepository[A, string](el)
	bS := memory.NewEventLogRepository[B, string](el)
	bS.Options.CompressEventLog = true

	a := &A{V: "42"}
	assert.Must(t).Nil(aS.Create(ctx, a))
	a.V = "24"
	assert.Must(t).Nil(aS.Update(ctx, a))
	assert.Must(t).Nil(aS.DeleteByID(ctx, a.ID))
	assert.Must(t).Equal(len(el.Events()), 3)

	b := &B{V: "4242"}
	assert.Must(t).Nil(bS.Create(ctx, b))
	assert.Must(t).Equal(len(el.Events()), 4)
	b.V = "2424"
	assert.Must(t).Nil(bS.Update(ctx, b))
	assert.Must(t).Equal(len(el.Events()), 4)
	assert.Must(t).Nil(bS.DeleteByID(ctx, b.ID))
	assert.Must(t).Equal(len(el.Events()), 3)

	aS.Compress()
	assert.Must(t).Equal(len(el.Events()), 0, `when events are compressed, the event log should be empty`)
}

func TestEventLogRepository_LookupTx(t *testing.T) {
	s := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())

	t.Run(`when outside of comproto`, func(t *testing.T) {
		_, ok := s.EventLog.LookupTx(context.Background())
		assert.Must(t).False(ok)
	})

	t.Run(`when during comproto`, func(t *testing.T) {
		ctx, err := s.BeginTx(context.Background())
		assert.Must(t).Nil(err)
		defer func() { _ = s.RollbackTx(ctx) }()

		e := TestEntity{Data: `42`}
		assert.Must(t).Nil(s.Create(ctx, &e))
		_, found, err := s.FindByID(ctx, e.ID)
		assert.Must(t).Nil(err)
		assert.Must(t).True(found)
		_, found, err = s.FindByID(context.Background(), e.ID)
		assert.Must(t).Nil(err)
		assert.Must(t).False(found)

		_, ok := s.EventLog.LookupTx(ctx)
		assert.Must(t).True(ok)
		_, ok = s.View(ctx)[e.ID]
		assert.Must(t).True(ok)
	})
}

func TestEventLogRepository_SaveEntityWithCustomKeyType(t *testing.T) {

	repository := memory.NewEventLogRepository[EntityWithStructID, StructID](memory.NewEventLog())
	var counter int
	repository.MakeID = func(ctx context.Context) (StructID, error) {
		counter++
		var id StructID
		id.V = counter
		return id, nil
	}

	makeEntityWithStructID := func(tb testing.TB) EntityWithStructID {
		t := tb.(*testcase.T)
		return EntityWithStructID{Data: t.Random.String()}
	}

	contracts := getRepositorySpecsForT[EntityWithStructID, StructID](repository, makeContext, makeEntityWithStructID)

	testcase.RunSuite(t, contracts...)
}

type StructID struct {
	V int
}

type EntityWithStructID struct {
	ID   StructID `ext:"ID"`
	Data string
}

func TestEventLogRepository_implementsCacheEntityRepository(t *testing.T) {
	testcase.RunSuite(t, cachecontracts.EntityRepository[TestEntity, string](func(tb testing.TB) cachecontracts.EntityRepositorySubject[TestEntity, string] {
		eventLog := memory.NewEventLog()
		repository := memory.NewEventLogRepository[TestEntity, string](eventLog)
		memory.LogHistoryOnFailure(tb, eventLog)
		return cachecontracts.EntityRepositorySubject[TestEntity, string]{
			EntityRepository: repository,
			CommitManager:    eventLog,
			MakeContext:      context.Background,
			MakeEntity:       func() TestEntity { return makeTestEntity(tb) },
		}
	}))
}

func TestEventLogRepository_multipleRepositoryForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	eventLog := memory.NewEventLog()
	s1 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#A")
	s2 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	crudtest.Create[TestEntity, string](t, s1, ctx, &ent)
	crudtest.IsAbsent[TestEntity, string](t, s2, ctx, crudtest.HasID[TestEntity, string](t, ent))
}

func TestEventLogRepository_contracts(t *testing.T) {
	s := testcase.NewSpec(t)
	type Entity struct {
		ID      string `ext:"id"`
		X, Y, Z string
	}
	makeEntity := func(tb testing.TB) Entity {
		t := tb.(*testcase.T)
		return Entity{
			X: t.Random.String(),
			Y: t.Random.String(),
			Z: t.Random.String(),
		}
	}
	resource.Contract[Entity, string](func(tb testing.TB) resource.ContractSubject[Entity, string] {
		el := memory.NewEventLog()
		stg := memory.NewEventLogRepository[Entity, string](el)
		return resource.ContractSubject[Entity, string]{
			Resource:      stg,
			MetaAccessor:  el,
			CommitManager: el,
			MakeContext:   testent.MakeContextFunc(tb),
			MakeEntity:    func() Entity { return makeEntity(tb) },
		}
	}).Spec(s)
}
