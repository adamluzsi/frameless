package crudcontracts

import (
	"context"
	"fmt"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
	"github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type OnePhaseCommitProtocol[Entity, ID any] struct {
	MakeSubject func(testing.TB) OnePhaseCommitProtocolSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type OnePhaseCommitProtocolSubject[Entity, ID any] struct {
	Resource      spechelper.CRD[Entity, ID]
	CommitManager comproto.OnePhaseCommitProtocol
}

func (c OnePhaseCommitProtocol[Entity, ID]) subject() testcase.Var[OnePhaseCommitProtocolSubject[Entity, ID]] {
	return testcase.Var[OnePhaseCommitProtocolSubject[Entity, ID]]{
		ID: "OnePhaseCommitProtocolSubject",
		Init: func(t *testcase.T) OnePhaseCommitProtocolSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) manager() testcase.Var[comproto.OnePhaseCommitProtocol] {
	return testcase.Var[comproto.OnePhaseCommitProtocol]{
		ID: "commit protocol manager",
		Init: func(t *testcase.T) comproto.OnePhaseCommitProtocol {
			return c.subject().Get(t).CommitManager
		},
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) resource() testcase.Var[spechelper.CRD[Entity, ID]] {
	return testcase.Var[spechelper.CRD[Entity, ID]]{
		ID: "managed resource",
		Init: func(t *testcase.T) spechelper.CRD[Entity, ID] {
			return c.subject().Get(t).Resource
		},
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c OnePhaseCommitProtocol[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c OnePhaseCommitProtocol[Entity, ID]) Spec(s *testcase.Spec) {
	s.HasSideEffect()

	// clean ahead before testing suite
	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() { spechelper.TryCleanup(t, c.MakeContext(t), c.resource().Get(t)) })
	})

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			ptr := spechelper.ToPtr(c.MakeEntity(t))
			Create[Entity, ID](t, c.resource().Get(t), tx, ptr)
			id := HasID[Entity, ID](t, ptr)

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = c.resource().Get(t).FindByID(tx, id)
			t.Must.NotNil(err)
			t.Must.NotNil(c.resource().Get(t).Create(tx, spechelper.ToPtr(c.MakeEntity(t))))

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(tx).Err())
			}

			if updater, ok := c.resource().Get(t).(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(tx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						c.resource().Get(t)))
			}

			t.Must.NotNil(c.resource().Get(t).DeleteByID(tx, id))
			if allDeleter, ok := c.resource().Get(t).(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(tx))
			}

			Waiter.Wait()
		})

		s.Test(`BeginTx+RollbackTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			ptr := spechelper.ToPtr(c.MakeEntity(t))
			t.Must.NoError(c.resource().Get(t).Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Must.NoError(c.manager().Get(t).RollbackTx(ctx))

			_, _, err = c.resource().Get(t).FindByID(ctx, id)
			t.Must.NotNil(err)

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(ctx).Err())
			}

			t.Must.NotNil(c.resource().Get(t).Create(ctx, spechelper.ToPtr(c.MakeEntity(t))))

			if updater, ok := c.resource().Get(t).(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						c.resource().Get(t)))
			}

			t.Must.NotNil(c.resource().Get(t).DeleteByID(ctx, id))

			if allDeleter, ok := c.resource().Get(t).(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(ctx))
			}
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)

			entity := spechelper.ToPtr(c.MakeEntity(t))
			Create[Entity, ID](t, c.resource().Get(t), tx, entity)
			id := HasID[Entity, ID](t, entity)

			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)             // can be found in tx Context
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id) // is absent from the global Context

			t.Must.Nil(c.manager().Get(t).CommitTx(tx)) // after the commit

			actually := IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)
			t.Must.Equal(entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			entity := spechelper.ToPtr(c.MakeEntity(t))
			//t.Must.Nil( Spec.resource().Get(t).Create(tx, entity))
			Create[Entity, ID](t, c.resource().Get(t), tx, entity)

			id := HasID[Entity, ID](t, entity)
			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)

			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))

			IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			entity := spechelper.ToPtr(c.MakeEntity(t))

			Create[Entity, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Entity, ID](t, entity)
			t.Defer(c.resource().Get(t).DeleteByID, ctx, id)

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)

			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Entity, ID](t, c.resource().Get(t), tx, id)

			// in global Context it is findable
			IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			entity := spechelper.ToPtr(c.MakeEntity(t))
			Create[Entity, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Entity, ID](t, entity)

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Entity, ID](t, c.resource().Get(t), tx, id)
			IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)
			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
			IsFindable[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(c.manager().Get(t).CommitTx(ctx))
			t.Must.NotNil(c.manager().Get(t).CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(c.manager().Get(t).RollbackTx(ctx))
			t.Must.NotNil(c.manager().Get(t).RollbackTx(ctx))
		})

		s.Test(`BeginTx should be callable multiple times to ensure emulate multi level transaction`, func(t *testcase.T) {
			t.Log(
				`Even if the current driver or resource don't support multi level transactions`,
				`It should still accept multiple transaction begin for a given Context`,
				`The benefit of this is that low level components that needs to ensure transactional execution,`,
				`they should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor itself.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			t.Defer(DeleteAll[Entity, ID], t, c.resource().Get(t), c.MakeContext(t))

			var globalContext = c.MakeContext(t)

			tx1, err := c.manager().Get(t).BeginTx(globalContext)
			t.Must.Nil(err)
			t.Log(`given tx1 is began`)

			e1 := spechelper.ToPtr(c.MakeEntity(t))
			t.Must.Nil(c.resource().Get(t).Create(tx1, e1))
			IsFindable[Entity, ID](t, c.resource().Get(t), tx1, HasID[Entity, ID](t, e1))
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e1))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := c.manager().Get(t).BeginTx(tx1)
			t.Must.Nil(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := spechelper.ToPtr(c.MakeEntity(t))
			t.Must.Nil(c.resource().Get(t).Create(tx2InTx1, e2))
			IsFindable[Entity, ID](t, c.resource().Get(t), tx2InTx1, HasID[Entity, ID](t, e2))    // tx2 can see e2
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e2)) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e1))
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e2))

			t.Must.Nil(c.manager().Get(t).CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.Nil(c.manager().Get(t).CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e1))
			IsFindable[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, e2))
		})

		s.Describe(`Publisher`, c.specPublisher)

		s.Describe(`.Purger`, c.specPurger)
	})
}

func (c OnePhaseCommitProtocol[Entity, ID]) specPurger(s *testcase.Spec) {
	purger := func(t *testcase.T) PurgerSubject[Entity, ID] {
		p, ok := c.resource().Get(t).(PurgerSubject[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply contract.PurgerSubject`, c.resource().Get(t))
		}
		return p
	}

	s.Before(func(t *testcase.T) { purger(t) }) // guard clause

	s.Test(`entity created prior to transaction won't be affected by a purge after a rollback`, func(t *testcase.T) {
		ptr := spechelper.ToPtr(c.MakeEntity(t))
		Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))

		t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))
	})

	s.Test(`entity created prior to transaction will be removed by a purge after the commit`, func(t *testcase.T) {
		ptr := spechelper.ToPtr(c.MakeEntity(t))
		Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))

		t.Must.Nil(c.manager().Get(t).CommitTx(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, ptr))
	})
}

func (c OnePhaseCommitProtocol[Entity, ID]) specPublisher(s *testcase.Spec) {
	s.Context(`Creator`, c.specCreatorPublisher)
	s.Context(`Updater`, c.specUpdaterPublisher)
	s.Context(`Deleter`, c.specDeleterPublisher)
}

func (c OnePhaseCommitProtocol[Entity, ID]) specCreatorPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) pubsubcontracts.CreatorPublisherSubject[Entity, ID] {
		p, ok := c.resource().Get(t).(pubsubcontracts.CreatorPublisherSubject[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Publisher and Creator`, c.resource().Get(t))
		}
		return p
	}

	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Entity, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.CreateEvent[Entity])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := testcase.Let(s, func(t *testcase.T) []*Entity {
			return spechelper.GenEntities[Entity](t, c.MakeEntity)
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToCreatorEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}
		subscription := spechelper.LetSubscription[Entity, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the repository`)
			for _, entity := range events.Get(t) {
				Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))

			var es []pubsub.CreateEvent[Entity]
			for _, ent := range events.Get(t) {
				es = append(es, pubsub.CreateEvent[Entity]{Entity: *ent})
			}
			Eventually.Assert(t, func(it assert.It) {
				it.Must.ContainExactly(es, subscriber.Get(t).CreateEvents())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
		})
	})
}

func (c OnePhaseCommitProtocol[Entity, ID]) specUpdaterPublisher(s *testcase.Spec) {
	updater := func(t *testcase.T) UpdaterSubject[Entity, ID] {
		u, ok := c.resource().Get(t).(UpdaterSubject[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply resources.Updater`, c.resource().Get(t))
		}
		return u
	}

	updaterPublisher := func(t *testcase.T) pubsubcontracts.UpdaterPublisherSubject[Entity, ID] {
		u, ok := c.resource().Get(t).(pubsubcontracts.UpdaterPublisherSubject[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Updater+Publisher`, c.resource().Get(t))
		}
		return u
	}

	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Entity, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.UpdateEvent[Entity])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := testcase.Let(s, func(t *testcase.T) []*Entity {
			return spechelper.GenEntities[Entity](t, c.MakeEntity)
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return updaterPublisher(t).SubscribeToUpdaterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}
		subscription := spechelper.LetSubscription[Entity, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the repository outside of the current transaction`)
			for _, ptr := range events.Get(t) {
				Create[Entity, ID](t, c.resource().Get(t), c.MakeContext(t), ptr)
			}

			t.Log(`then events being updated`)
			for _, ptr := range events.Get(t) {
				Update[Entity, ID](t, updater(t), spechelper.ContextVar.Get(t), ptr)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))

			var es []pubsub.UpdateEvent[Entity]
			for _, ent := range events.Get(t) {
				es = append(es, pubsub.UpdateEvent[Entity]{Entity: *ent})
			}
			Eventually.Assert(t, func(tb assert.It) {
				tb.Must.ContainExactly(es, subscriber.Get(t).UpdateEvents())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
		})
	})
}

func (c OnePhaseCommitProtocol[Entity, ID]) specDeleterPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) pubsubcontracts.DeleterPublisherSubject[Entity, ID] {
		u, ok := c.resource().Get(t).(pubsubcontracts.DeleterPublisherSubject[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Deleter+Publisher`, c.resource().Get(t))
		}
		return u
	}

	s.Describe(`#SubscribeToDeleteByID`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Entity, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.DeleteByIDEvent[ID])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := testcase.Let(s, func(t *testcase.T) *Entity {
			return spechelper.ToPtr(c.MakeEntity(t))
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}

		hasDeleteEntity := pubsubcontracts.DeleterPublisher[Entity, ID]{}.HasDeleteEntity
		subscription := spechelper.LetSubscription[Entity, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`given entity already created during the transaction`)
			Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			t.Log(`and then the entity is also deleted during the transaction`)
			Delete[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))
		})

		s.Then(`before a commit, delete events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
		})

		s.Then(`after a commit, delete events will arrive to the subscriber`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
			Eventually.Assert(t, func(tb assert.It) {
				tb.Must.False(subscriber.Get(t).EventsLen() < 1)
			})

			hasDeleteEntity(t, subscriber.Get(t).Events, pubsub.DeleteByIDEvent[ID]{ID: HasID[Entity, ID](t, entity.Get(t))})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			t.Must.Empty(subscriber.Get(t).Events())
		})
	})

	s.Describe(`#SubscribeToDeleteAll`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Entity, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.DeleteAllEvent)
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeContext(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := testcase.Let(s, func(t *testcase.T) *Entity {
			return spechelper.ToPtr(c.MakeEntity(t))
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		subscription := spechelper.LetSubscription[Entity, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub, err := subject(t)
			t.Must.Nil(err)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			_ = entity
			t.Log(`given entity already created`)
			// TODO: why this makes a DeleteAll event somehow?
			Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			allDeleter, ok := c.resource().Get(t).(crud.AllDeleter)
			if !ok {
				t.Skipf("crud.AllDeleter is not supported by %T", c.resource().Get(t))
			}

			t.Log(`and then the entity is also deleted`)
			t.Must.NoError(allDeleter.DeleteAll(spechelper.ContextVar.Get(t)))
			Eventually.Assert(t, func(it assert.It) {
				IsAbsent[Entity, ID](it, c.resource().Get(t), c.MakeContext(t), HasID[Entity, ID](it, entity.Get(t)))
			})
		})

		s.Then(`before a commit, deleteAll events will be absent`, func(t *testcase.T) {
			t.Must.Empty(subscriber.Get(t).Events())
		})

		s.Then(`after a commit, delete all event will arrive to the subscriber`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
			Eventually.Assert(t, func(tb assert.It) {
				tb.Must.True(subscriber.Get(t).EventsLen() == 1, `one event was expected, but didn't arrived`)
				tb.Must.Contain(subscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
			})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			t.Must.Empty(subscriber.Get(t).Events())
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
		})
	})
}
