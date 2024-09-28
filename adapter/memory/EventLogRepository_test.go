package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/meta/metacontracts"
	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
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
	m := memory.NewEventLog()
	repo := memory.NewEventLogRepository[TestEntity, string](m)

	testcase.RunSuite(t, resource.Contract[TestEntity, string](repo, resource.Config[TestEntity, string]{
		CRUD:          crudcontracts.Config[TestEntity, string]{MakeEntity: makeTestEntity},
		MetaAccessor:  m,
		CommitManager: repo,
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

	assert.Nil(t, subject.Create(ctx, &TestEntity{Data: `A`}))
	assert.Nil(t, subject.Create(ctx, &TestEntity{Data: `B`}))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(ctx)))
	assert.Nil(t, err)
	assert.Must(t).Equal(2, count)

	assert.Nil(t, subject.DeleteAll(ctx))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(ctx)))
	assert.Nil(t, err)
	assert.Must(t).Equal(0, count)

	tx1CTX, err := subject.BeginTx(ctx)
	assert.Nil(t, err)
	assert.Nil(t, subject.Create(tx1CTX, &TestEntity{Data: `C`}))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(tx1CTX)))
	assert.Nil(t, err)
	assert.Must(t).Equal(1, count)
	assert.Nil(t, subject.RollbackTx(tx1CTX))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(ctx)))
	assert.Nil(t, err)
	assert.Must(t).Equal(0, count)

	tx2CTX, err := subject.BeginTx(ctx)
	assert.Nil(t, err)
	assert.Nil(t, subject.Create(tx2CTX, &TestEntity{Data: `D`}))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(tx2CTX)))
	assert.Nil(t, err)
	assert.Must(t).Equal(1, count)
	assert.Nil(t, subject.CommitTx(tx2CTX))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(ctx)))
	assert.Nil(t, err)
	assert.Must(t).Equal(1, count)

	assert.Nil(t, subject.DeleteAll(ctx))
	count, err = iterators.Count(iterators.WithErr(subject.FindAll(ctx)))
	assert.Nil(t, err)
	assert.Must(t).Equal(0, count)
}

func getRepositorySpecsForT[Entity any, ID comparable](
	subject *memory.EventLogRepository[Entity, ID],
	MakeContext func(testing.TB) context.Context,
	MakeEntity func(testing.TB) Entity,
) []testcase.Suite {
	crudConfig := crudcontracts.Config[Entity, ID]{
		MakeContext:     MakeContext,
		MakeEntity:      MakeEntity,
		SupportIDReuse:  true,
		SupportRecreate: false,
	}
	cacheConfig := cachecontracts.Config[Entity, ID]{
		CRUD: crudConfig,
	}
	metaConfig := metacontracts.Config[int]{
		MakeV: func(tb testing.TB) int { return testcase.ToT(&tb).Random.Int() },
	}
	return []testcase.Suite{
		crudcontracts.Creator[Entity, ID](subject, crudConfig),
		crudcontracts.Finder[Entity, ID](subject, crudConfig),
		crudcontracts.Updater[Entity, ID](subject, crudConfig),
		crudcontracts.Deleter[Entity, ID](subject, crudConfig),
		crudcontracts.OnePhaseCommitProtocol[Entity, ID](subject, subject.EventLog, crudConfig),
		cachecontracts.EntityRepository[Entity, ID](subject, subject.EventLog, cacheConfig),
		metacontracts.MetaAccessor[int](subject.EventLog, metaConfig),
	}
}

func getRepositorySpecs[Entity any, ID comparable](
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

		bctx := context.Background()
		txS1, err := subject1.BeginTx(bctx)
		assert.Nil(t, err)
		txS2, err := subject2.BeginTx(txS1)
		assert.Nil(t, err)

		t.Log(`when in subject 1 store an entity`)
		entity := &TestEntity{Data: `42`}
		assert.Nil(t, subject1.Create(txS1, entity))

		t.Log(`and subject 2 finish comproto`)
		assert.Nil(t, subject2.CommitTx(txS2))
		t.Log(`and subject 2 then try to find this entity`)
		_, found, err := subject2.FindByID(bctx, entity.ID)
		assert.Nil(t, err)
		assert.Must(t).False(found, `it should not see the uncommitted entity`)

		t.Log(`but after subject 1 commit the comproto`)
		assert.Nil(t, subject1.CommitTx(txS1))
		t.Log(`subject 1 can see the newT entity`)
		_, found, err = subject1.FindByID(bctx, entity.ID)
		assert.Nil(t, err)
		assert.True(t, found)
	})

	t.Run(`deletes across comproto instances in the same context`, func(t *testing.T) {
		subject1 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		subject2 := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())

		bctx := context.Background()
		e1 := rnd.Make(TestEntity{}).(TestEntity)
		e1.ID = ""
		e2 := rnd.Make(TestEntity{}).(TestEntity)
		e2.ID = ""

		assert.Nil(t, subject1.Create(bctx, &e1))
		id1, ok := extid.Lookup[string](e1)
		assert.True(t, ok)
		assert.NotEmpty(t, id1)
		t.Cleanup(func() { _ = subject1.DeleteByID(context.Background(), id1) })

		assert.Nil(t, subject2.Create(bctx, &e2))
		id2, ok := extid.Lookup[string](e2)
		assert.True(t, ok)
		assert.NotEmpty(t, id2)
		t.Cleanup(func() { _ = subject2.DeleteByID(context.Background(), id2) })

		tx1, err := subject1.BeginTx(bctx)
		assert.Nil(t, err)
		tx2, err := subject2.BeginTx(tx1)
		assert.Nil(t, err)

		_, found, err := subject1.FindByID(tx1, id1)
		assert.Nil(t, err)
		assert.True(t, found)
		assert.Nil(t, subject1.DeleteByID(tx1, id1))

		_, found, err = subject2.FindByID(tx2, id2)
		assert.NoError(t, err)
		assert.True(t, found)
		_, found, err = subject2.FindByID(tx1, id2)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Nil(t, subject2.DeleteByID(tx2, id2))

		_, found, err = subject1.FindByID(tx1, id1)
		assert.Nil(t, err)
		assert.Must(t).False(found)

		_, found, err = subject2.FindByID(tx2, id2)
		assert.Nil(t, err)
		assert.Must(t).False(found)

		_, found, err = subject1.FindByID(bctx, id1)
		assert.Nil(t, err)
		assert.True(t, found)

		assert.Nil(t, subject2.CommitTx(tx2))
		assert.Nil(t, subject1.CommitTx(tx1))

		_, found, err = subject1.FindByID(bctx, id1)
		assert.Nil(t, err)
		assert.False(t, found)
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

	m.Compress()
	assert.Must(t).Empty(m.Events(),
		`after all the specs, the memory repository was expected to be empty.`+
			` If the repository has values, it means something is not cleaning up properly in the specs.`)
}

func TestEventLogRepository_NewIDFunc(t *testing.T) {
	t.Run(`when NewID is absent`, func(t *testing.T) {
		repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		repository.MakeID = nil

		ptr := &TestEntity{Data: "42"}
		assert.Nil(t, repository.Create(context.Background(), ptr))
		assert.NotEmpty(t, ptr.ID)
	})

	t.Run(`when NewID is provided`, func(t *testing.T) {
		repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		expectedID := random.New(random.CryptoSeed{}).String()
		repository.MakeID = func(ctx context.Context) (string, error) {
			return expectedID, nil
		}

		ptr := &TestEntity{Data: "42"}
		assert.Nil(t, repository.Create(context.Background(), ptr))
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
	assert.Nil(t, aS.Create(ctx, a))
	a.V = "24"
	assert.Nil(t, aS.Update(ctx, a))
	assert.Nil(t, aS.DeleteByID(ctx, a.ID))
	assert.Must(t).Equal(len(el.Events()), 3)

	b := &B{V: "4242"}
	assert.Nil(t, bS.Create(ctx, b))
	assert.Must(t).Equal(len(el.Events()), 4)
	b.V = "2424"
	assert.Nil(t, bS.Update(ctx, b))
	assert.Must(t).Equal(len(el.Events()), 4)
	assert.Nil(t, bS.DeleteByID(ctx, b.ID))
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
		assert.Nil(t, err)
		defer func() { _ = s.RollbackTx(ctx) }()

		e := TestEntity{Data: `42`}
		assert.Nil(t, s.Create(ctx, &e))
		_, found, err := s.FindByID(ctx, e.ID)
		assert.Nil(t, err)
		assert.True(t, found)
		_, found, err = s.FindByID(context.Background(), e.ID)
		assert.Nil(t, err)
		assert.Must(t).False(found)

		_, ok := s.EventLog.LookupTx(ctx)
		assert.True(t, ok)
		_, ok = s.View(ctx)[e.ID]
		assert.True(t, ok)
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
	eventLog := memory.NewEventLog()
	repository := memory.NewEventLogRepository[TestEntity, string](eventLog)
	memory.LogHistoryOnFailure(t, eventLog)
	testcase.RunSuite(t, cachecontracts.EntityRepository[TestEntity, string](repository, eventLog))
}

func TestEventLogRepository_multipleRepositoryForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	eventLog := memory.NewEventLog()
	s1 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#A")
	s2 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	crudtest.Create[TestEntity, string](t, s1, ctx, &ent)
	crudtest.IsAbsent[TestEntity, string](t, s2, ctx, ent.ID)
}

func TestEventLogRepository_contracts(t *testing.T) {
	s := testcase.NewSpec(t)
	type Entity struct {
		ID      string `ext:"id"`
		X, Y, Z string
	}
	el := memory.NewEventLog()
	stg := memory.NewEventLogRepository[Entity, string](el)
	resource.Contract[Entity, string](stg, resource.Config[Entity, string]{
		MetaAccessor:  el,
		CommitManager: el,
	}).Spec(s)
}
