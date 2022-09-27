package crudcontracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type OnePhaseCommitProtocol[Ent, ID any] struct {
	Subject func(testing.TB) OnePhaseCommitProtocolSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type OnePhaseCommitProtocolSubject[Ent, ID any] struct {
	Resource      spechelper.CRD[Ent, ID]
	CommitManager comproto.OnePhaseCommitProtocol
}

func (c OnePhaseCommitProtocol[Ent, ID]) subject() testcase.Var[OnePhaseCommitProtocolSubject[Ent, ID]] {
	return testcase.Var[OnePhaseCommitProtocolSubject[Ent, ID]]{
		ID: "OnePhaseCommitProtocolSubject",
		Init: func(t *testcase.T) OnePhaseCommitProtocolSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c OnePhaseCommitProtocol[Ent, ID]) manager() testcase.Var[comproto.OnePhaseCommitProtocol] {
	return testcase.Var[comproto.OnePhaseCommitProtocol]{
		ID: "commit protocol manager",
		Init: func(t *testcase.T) comproto.OnePhaseCommitProtocol {
			return c.subject().Get(t).CommitManager
		},
	}
}

func (c OnePhaseCommitProtocol[Ent, ID]) resource() testcase.Var[spechelper.CRD[Ent, ID]] {
	return testcase.Var[spechelper.CRD[Ent, ID]]{
		ID: "managed resource",
		Init: func(t *testcase.T) spechelper.CRD[Ent, ID] {
			return c.subject().Get(t).Resource
		},
	}
}

func (c OnePhaseCommitProtocol[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c OnePhaseCommitProtocol[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c OnePhaseCommitProtocol[Ent, ID]) Spec(s *testcase.Spec) {
	s.HasSideEffect()

	// clean ahead before testing suite
	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() { spechelper.TryCleanup(t, c.MakeCtx(t), c.resource().Get(t)) })
	})

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			ptr := spechelper.ToPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), tx, ptr)
			id := HasID[Ent, ID](t, ptr)

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = c.resource().Get(t).FindByID(tx, id)
			t.Must.NotNil(err)
			t.Must.NotNil(c.resource().Get(t).Create(tx, spechelper.ToPtr(c.MakeEnt(t))))

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Ent, ID]); ok {
				t.Must.NotNil(allFinder.FindAll(tx).Err())
			}

			if updater, ok := c.resource().Get(t).(crud.Updater[Ent]); ok {
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
		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.MakeCtx(t)
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			ptr := spechelper.ToPtr(c.MakeEnt(t))
			t.Must.Nil(c.resource().Get(t).Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Must.Nil(c.manager().Get(t).RollbackTx(ctx))

			_, _, err = c.resource().Get(t).FindByID(ctx, id)
			t.Must.NotNil(err)

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Ent, ID]); ok {
				t.Must.NotNil(allFinder.FindAll(ctx).Err())
			}

			t.Must.NotNil(c.resource().Get(t).Create(ctx, spechelper.ToPtr(c.MakeEnt(t))))

			if updater, ok := c.resource().Get(t).(crud.Updater[Ent]); ok {
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
			tx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)

			entity := spechelper.ToPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), tx, entity)
			id := HasID[Ent, ID](t, entity)

			IsFindable[Ent, ID](t, c.resource().Get(t), tx, id)         // can be found in tx Context
			IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id) // is absent from the global Context

			t.Must.Nil(c.manager().Get(t).CommitTx(tx)) // after the commit

			actually := IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)
			t.Must.Equal(entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			entity := spechelper.ToPtr(c.MakeEnt(t))
			//t.Must.Nil( Spec.resource().Get(t).Create(tx, entity))
			Create[Ent, ID](t, c.resource().Get(t), tx, entity)

			id := HasID[Ent, ID](t, entity)
			IsFindable[Ent, ID](t, c.resource().Get(t), tx, id)
			IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)

			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))

			IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeCtx(t)
			entity := spechelper.ToPtr(c.MakeEnt(t))

			Create[Ent, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Ent, ID](t, entity)
			t.Defer(c.resource().Get(t).DeleteByID, ctx, id)

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)

			IsFindable[Ent, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Ent, ID](t, c.resource().Get(t), tx, id)

			// in global Context it is findable
			IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))
			IsAbsent[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeCtx(t)
			entity := spechelper.ToPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Ent, ID](t, entity)

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			IsFindable[Ent, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Ent, ID](t, c.resource().Get(t), tx, id)
			IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)
			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
			IsFindable[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeCtx(t)
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(c.manager().Get(t).CommitTx(ctx))
			t.Must.NotNil(c.manager().Get(t).CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeCtx(t)
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

			t.Defer(DeleteAll[Ent, ID], t, c.resource().Get(t), c.MakeCtx(t))

			var globalContext = c.MakeCtx(t)

			tx1, err := c.manager().Get(t).BeginTx(globalContext)
			t.Must.Nil(err)
			t.Log(`given tx1 is began`)

			e1 := spechelper.ToPtr(c.MakeEnt(t))
			t.Must.Nil(c.resource().Get(t).Create(tx1, e1))
			IsFindable[Ent, ID](t, c.resource().Get(t), tx1, HasID[Ent, ID](t, e1))
			IsAbsent[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e1))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := c.manager().Get(t).BeginTx(tx1)
			t.Must.Nil(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := spechelper.ToPtr(c.MakeEnt(t))
			t.Must.Nil(c.resource().Get(t).Create(tx2InTx1, e2))
			IsFindable[Ent, ID](t, c.resource().Get(t), tx2InTx1, HasID[Ent, ID](t, e2))    // tx2 can see e2
			IsAbsent[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e2)) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e1))
			IsAbsent[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e2))

			t.Must.Nil(c.manager().Get(t).CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.Nil(c.manager().Get(t).CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e1))
			IsFindable[Ent, ID](t, c.resource().Get(t), globalContext, HasID[Ent, ID](t, e2))
		})

		s.Describe(`Publisher`, c.specPublisher)

		s.Describe(`.Purger`, c.specPurger)
	})
}

func (c OnePhaseCommitProtocol[Ent, ID]) specPurger(s *testcase.Spec) {
	purger := func(t *testcase.T) PurgerSubject[Ent, ID] {
		p, ok := c.resource().Get(t).(PurgerSubject[Ent, ID])
		if !ok {
			t.Skipf(`%T doesn't supply contract.PurgerSubject`, c.resource().Get(t))
		}
		return p
	}

	s.Before(func(t *testcase.T) { purger(t) }) // guard clause

	s.Test(`entity created prior to transaction won't be affected by a purge after a rollback`, func(t *testcase.T) {
		ptr := spechelper.ToPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))
		IsFindable[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))

		t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
		IsFindable[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))
	})

	s.Test(`entity created prior to transaction will be removed by a purge after the commit`, func(t *testcase.T) {
		ptr := spechelper.ToPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))
		IsFindable[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))

		t.Must.Nil(c.manager().Get(t).CommitTx(tx))
		IsAbsent[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Ent, ID](t, ptr))
	})
}

func (c OnePhaseCommitProtocol[Ent, ID]) specPublisher(s *testcase.Spec) {
	s.Context(`Creator`, c.specCreatorPublisher)
	s.Context(`Updater`, c.specUpdaterPublisher)
	s.Context(`Deleter`, c.specDeleterPublisher)
}

func (c OnePhaseCommitProtocol[Ent, ID]) specCreatorPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) pubsubcontracts.CreatorPublisherSubject[Ent, ID] {
		p, ok := c.resource().Get(t).(pubsubcontracts.CreatorPublisherSubject[Ent, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Publisher and Creator`, c.resource().Get(t))
		}
		return p
	}

	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Ent, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.CreateEvent[Ent])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := testcase.Let(s, func(t *testcase.T) []*Ent {
			return spechelper.GenEntities[Ent](t, c.MakeEnt)
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToCreatorEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}
		subscription := spechelper.LetSubscription[Ent, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the repository`)
			for _, entity := range events.Get(t) {
				Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))

			var es []pubsub.CreateEvent[Ent]
			for _, ent := range events.Get(t) {
				es = append(es, pubsub.CreateEvent[Ent]{Entity: *ent})
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

func (c OnePhaseCommitProtocol[Ent, ID]) specUpdaterPublisher(s *testcase.Spec) {
	updater := func(t *testcase.T) UpdaterSubject[Ent, ID] {
		u, ok := c.resource().Get(t).(UpdaterSubject[Ent, ID])
		if !ok {
			t.Skipf(`%T doesn't supply resources.Updater`, c.resource().Get(t))
		}
		return u
	}

	updaterPublisher := func(t *testcase.T) pubsubcontracts.UpdaterPublisherSubject[Ent, ID] {
		u, ok := c.resource().Get(t).(pubsubcontracts.UpdaterPublisherSubject[Ent, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Updater+Publisher`, c.resource().Get(t))
		}
		return u
	}

	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Ent, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.UpdateEvent[Ent])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := testcase.Let(s, func(t *testcase.T) []*Ent {
			return spechelper.GenEntities[Ent](t, c.MakeEnt)
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return updaterPublisher(t).SubscribeToUpdaterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}
		subscription := spechelper.LetSubscription[Ent, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the repository outside of the current transaction`)
			for _, ptr := range events.Get(t) {
				Create[Ent, ID](t, c.resource().Get(t), c.MakeCtx(t), ptr)
			}

			t.Log(`then events being updated`)
			for _, ptr := range events.Get(t) {
				Update[Ent, ID](t, updater(t), spechelper.ContextVar.Get(t), ptr)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			t.Must.Empty(subscriber.Get(t).Events())
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).CommitTx(spechelper.ContextVar.Get(t)))

			var es []pubsub.UpdateEvent[Ent]
			for _, ent := range events.Get(t) {
				es = append(es, pubsub.UpdateEvent[Ent]{Entity: *ent})
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

func (c OnePhaseCommitProtocol[Ent, ID]) specDeleterPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) pubsubcontracts.DeleterPublisherSubject[Ent, ID] {
		u, ok := c.resource().Get(t).(pubsubcontracts.DeleterPublisherSubject[Ent, ID])
		if !ok {
			t.Skipf(`%T doesn't supply frameless Deleter+Publisher`, c.resource().Get(t))
		}
		return u
	}

	s.Describe(`#SubscribeToDeleteByID`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Ent, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.DeleteByIDEvent[ID])
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := testcase.Let(s, func(t *testcase.T) *Ent {
			return spechelper.ToPtr(c.MakeEnt(t))
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}

		hasDeleteEntity := pubsubcontracts.DeleterPublisher[Ent, ID]{}.HasDeleteEntity
		subscription := spechelper.LetSubscription[Ent, ID](s)

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			t.Must.NotNil(sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`given entity already created during the transaction`)
			Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			t.Log(`and then the entity is also deleted during the transaction`)
			Delete[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))
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

			hasDeleteEntity(t, subscriber.Get(t).Events, pubsub.DeleteByIDEvent[ID]{ID: HasID[Ent, ID](t, entity.Get(t))})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			t.Must.Nil(c.manager().Get(t).RollbackTx(spechelper.ContextVar.Get(t)))
			t.Must.Empty(subscriber.Get(t).Events())
		})
	})

	s.Describe(`#SubscribeToDeleteAll`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Ent, ID](s, func(event interface{}) bool {
			_, ok := event.(pubsub.DeleteAllEvent)
			return ok
		})
		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.manager().Get(t).BeginTx(c.MakeCtx(t))
			t.Must.Nil(err)
			t.Defer(c.manager().Get(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := testcase.Let(s, func(t *testcase.T) *Ent {
			return spechelper.ToPtr(c.MakeEnt(t))
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		}
		subscription := spechelper.LetSubscription[Ent, ID](s)

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
			Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			allDeleter, ok := c.resource().Get(t).(crud.AllDeleter)
			if !ok {
				t.Skipf("crud.AllDeleter is not supported by %T", c.resource().Get(t))
			}

			t.Log(`and then the entity is also deleted`)
			t.Must.NoError(allDeleter.DeleteAll(spechelper.ContextVar.Get(t)))
			Eventually.Assert(t, func(it assert.It) {
				IsAbsent[Ent, ID](it, c.resource().Get(t), c.MakeCtx(t), HasID[Ent, ID](it, entity.Get(t)))
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
