package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/contracts"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"
	contracts2 "github.com/adamluzsi/frameless/spechelper/resource"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/crud/cache"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"

	cachecontracts "github.com/adamluzsi/frameless/ports/crud/cache/contracts"
	"github.com/adamluzsi/testcase"
)

var _ interface {
	crud.Creator[TestEntity]
	crud.Finder[TestEntity, string]
	crud.Updater[TestEntity]
	crud.Deleter[string]
	pubsub.CreatorPublisher[TestEntity]
	pubsub.UpdaterPublisher[TestEntity]
	pubsub.DeleterPublisher[string]
	comproto.OnePhaseCommitProtocol
} = &memory.EventLogRepository[TestEntity, string]{}

var _ cache.EntityRepository[TestEntity, string] = &memory.EventLogRepository[TestEntity, string]{}

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
	makeStringV := func(tb testing.TB) string {
		return tb.(*testcase.T).Random.String()
	}
	makeIntV := func(tb testing.TB) int {
		return tb.(*testcase.T).Random.Int()
	}
	return []testcase.Suite{
		crudcontracts.Creator[Entity, ID]{MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] { return subject }, MakeEntity: MakeEntity, MakeContext: MakeContext},
		crudcontracts.Finder[Entity, ID]{MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] { return subject }, MakeEntity: MakeEntity, MakeContext: MakeContext},
		crudcontracts.Updater[Entity, ID]{MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] { return subject }, MakeEntity: MakeEntity, MakeContext: MakeContext},
		crudcontracts.Deleter[Entity, ID]{MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] { return subject }, MakeEntity: MakeEntity, MakeContext: MakeContext},
		pubsubcontracts.Publisher[Entity, ID]{MakeSubject: func(tb testing.TB) pubsubcontracts.PublisherSubject[Entity, ID] { return subject }, MakeEntity: MakeEntity, MakeContext: MakeContext},
		crudcontracts.OnePhaseCommitProtocol[Entity, ID]{MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
			return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{Resource: subject, CommitManager: subject}
		}, MakeEntity: MakeEntity, MakeContext: MakeContext},
		cachecontracts.EntityRepository[Entity, ID]{MakeSubject: func(tb testing.TB) (repository cache.EntityRepository[Entity, ID], cpm comproto.OnePhaseCommitProtocol) {
			return subject, subject.EventLog
		}, MakeEntity: MakeEntity, MakeContext: MakeContext},
		frmetacontracts.MetaAccessor[Entity, ID, string]{
			MakeSubject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[Entity, ID, string] {
				return frmetacontracts.MetaAccessorSubject[Entity, ID, string]{
					MetaAccessor: subject.EventLog,
					Resource:     subject,
					Publisher:    subject,
				}
			},
			MakeEntity:  MakeEntity,
			MakeContext: MakeContext,
			MakeV:       makeStringV,
		},
		frmetacontracts.MetaAccessor[Entity, ID, int]{
			MakeSubject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[Entity, ID, int] {
				return frmetacontracts.MetaAccessorSubject[Entity, ID, int]{
					MetaAccessor: subject.EventLog,
					Resource:     subject,
					Publisher:    subject,
				}
			},
			MakeEntity:  MakeEntity,
			MakeContext: MakeContext,
			MakeV:       makeIntV,
		},
	}
}

func getRepositorySpecs[Entity, ID any](
	subject *memory.EventLogRepository[Entity, ID],
	MakeEntity func(testing.TB) Entity,
) []testcase.Suite {
	makeContext := func(testing.TB) context.Context { return context.Background() }
	return getRepositorySpecsForT[Entity, ID](subject, makeContext, MakeEntity)
}

func TestEventLogRepository(t *testing.T) {
	repository := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
	contracts := getRepositorySpecs[TestEntity](repository, makeTestEntity)
	testcase.RunSuite(t, contracts...)
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

func TestEventLogRepository_Options_AsyncSubscriptionHandling(t *testing.T) {
	s := testcase.NewSpec(t)

	hangingSubscriber := testcase.Let(s, func(t *testcase.T) *HangingSubscriber {
		return NewHangingSubscriber()
	})

	var subscriber = func(t *testcase.T) *HangingSubscriber { return hangingSubscriber.Get(t) }

	var newRepository = func(t *testcase.T) *memory.EventLogRepository[TestEntity, string] {
		s := memory.NewEventLogRepository[TestEntity, string](memory.NewEventLog())
		ctx := context.Background()
		subscription, err := s.SubscribeToCreate(ctx, subscriber(t))
		assert.Must(t).Nil(err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToUpdate(ctx, subscriber(t))
		assert.Must(t).Nil(err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteAll(ctx, subscriber(t))
		assert.Must(t).Nil(err)
		t.Defer(subscription.Close)
		subscription, err = s.SubscribeToDeleteByID(ctx, subscriber(t))
		assert.Must(t).Nil(err)
		t.Defer(subscription.Close)
		return s
	}

	disableAsyncSubscriptionHandling := testcase.Var[bool]{ID: "DisableAsyncSubscriptionHandling"}

	var subject = func(t *testcase.T) *memory.EventLogRepository[TestEntity, string] {
		s := newRepository(t)
		s.EventLog.Options.DisableAsyncSubscriptionHandling = disableAsyncSubscriptionHandling.Get(t)
		return s
	}

	s.Before(func(t *testcase.T) {
		if testing.Short() {
			t.Skip()
		}
	})

	const hangingDuration = 500 * time.Millisecond

	thenCreateUpdateDeleteWill := func(s *testcase.Spec, willHang bool) {
		var desc string
		if willHang {
			desc = `event is blocking until subscriber finishes handling the event`
		} else {
			desc = `event should not hang while the subscriber is busy`
		}
		desc = ` ` + desc

		var assertion = func(t testing.TB, expected, actual time.Duration) {
			if willHang {
				assert.Must(t).True(int64(expected) <= int64(actual))
			} else {
				assert.Must(t).True(int64(expected) > int64(actual))
			}
		}

		s.Then(`Create`+desc, func(t *testcase.T) {
			m := subject(t)
			sub := subscriber(t)

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			assert.Must(t).Nil(m.Create(context.Background(), &TestEntity{Data: `42`}))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`Update`+desc, func(t *testcase.T) {
			m := subject(t)
			sub := subscriber(t)

			ent := TestEntity{Data: `42`}
			assert.Must(t).Nil(m.Create(context.Background(), &ent))
			ent.Data = `foo`

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			assert.Must(t).Nil(m.Update(context.Background(), &ent))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`DeleteByID`+desc, func(t *testcase.T) {
			m := subject(t)
			sub := subscriber(t)

			ent := TestEntity{Data: `42`}
			assert.Must(t).Nil(m.Create(context.Background(), &ent))

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			assert.Must(t).Nil(m.DeleteByID(context.Background(), ent.ID))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Then(`DeleteAll`+desc, func(t *testcase.T) {
			m := subject(t)
			sub := subscriber(t)

			initialTime := time.Now()
			sub.HangFor(hangingDuration)
			assert.Must(t).Nil(m.DeleteAll(context.Background()))
			finishTime := time.Now()

			assertion(t, hangingDuration, finishTime.Sub(initialTime))
		})

		s.Test(`E2E`, func(t *testcase.T) {
			testcase.RunSuite(t, getRepositorySpecs[TestEntity, string](subject(t), makeTestEntity)...)
		})
	}

	s.When(`is enabled`, func(s *testcase.Spec) {
		disableAsyncSubscriptionHandling.LetValue(s, false)

		thenCreateUpdateDeleteWill(s, false)
	}, testcase.SkipBenchmark())

	s.When(`is disabled`, func(s *testcase.Spec) {
		disableAsyncSubscriptionHandling.LetValue(s, true)

		thenCreateUpdateDeleteWill(s, true)
	})
}

func NewHangingSubscriber() *HangingSubscriber {
	return &HangingSubscriber{}
}

type HangingSubscriber struct {
	m sync.RWMutex
}

func (h *HangingSubscriber) HangFor(d time.Duration) {
	h.m.Lock()
	go func() {
		defer h.m.Unlock()
		<-time.After(d)
	}()
}

func (h *HangingSubscriber) Handle(ctx context.Context, ent interface{}) error {
	h.m.RLock()
	defer h.m.RUnlock()
	return nil
}

func (h *HangingSubscriber) HandleError(ctx context.Context, err error) error {
	h.m.RLock()
	defer h.m.RUnlock()
	return nil
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
		defer s.RollbackTx(ctx)

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
	testcase.RunSuite(t, cachecontracts.EntityRepository[TestEntity, string]{
		MakeSubject: func(tb testing.TB) (cache.EntityRepository[TestEntity, string], comproto.OnePhaseCommitProtocol) {
			eventLog := memory.NewEventLog()
			repository := memory.NewEventLogRepository[TestEntity, string](eventLog)
			memory.LogHistoryOnFailure(tb, eventLog)
			return repository, eventLog
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeEntity: makeTestEntity,
	})
}

func TestEventLogRepository_Create_withAsyncSubscriptions(t *testing.T) {
	eventLog := memory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = false
	repository := memory.NewEventLogRepository[TestEntity, string](eventLog)
	ctx := context.Background()

	sub, err := repository.SubscribeToCreate(ctx, doubles.StubSubscriber[TestEntity, string]{})
	assert.Must(t).Nil(err)
	t.Cleanup(func() { assert.Must(t).Nil(sub.Close()) })

	ent := TestEntity{Data: random.New(random.CryptoSeed{}).StringN(4)}
	assert.Must(t).Nil(repository.Create(ctx, &ent))
	IsFindable[TestEntity, string](t, repository, ctx, ent.ID)
}

func TestEventLogRepository_multipleRepositoryForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	eventLog := memory.NewEventLog()
	s1 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#A")
	s2 := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	Create[TestEntity, string](t, s1, ctx, &ent)
	IsAbsent[TestEntity, string](t, s2, ctx, HasID[TestEntity, string](t, &ent))
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
	makeV := func(tb testing.TB) string {
		return tb.(*testcase.T).Random.String()
	}
	contracts2.Contract[Entity, string, string]{
		MakeSubject: func(tb testing.TB) contracts2.ContractSubject[Entity, string] {
			el := memory.NewEventLog()
			stg := memory.NewEventLogRepository[Entity, string](el)
			return contracts2.ContractSubject[Entity, string]{
				MetaAccessor:  el,
				CommitManager: el,
				Resource:      stg,
			}
		},
		MakeContext: makeContext,
		MakeEntity:  makeEntity,
		MakeV:       makeV,
	}.Spec(s)
}
