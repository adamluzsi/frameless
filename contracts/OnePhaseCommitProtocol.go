package contracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type OnePhaseCommitProtocol struct {
	T
	Subject        func(testing.TB) (frameless.OnePhaseCommitProtocol, CRD)
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

func (c OnePhaseCommitProtocol) manager() testcase.Var {
	return testcase.Var{Name: "commit protocol manager"}
}

func (c OnePhaseCommitProtocol) managerGet(t *testcase.T) frameless.OnePhaseCommitProtocol {
	return c.manager().Get(t).(frameless.OnePhaseCommitProtocol)
}

func (c OnePhaseCommitProtocol) resource() testcase.Var {
	return testcase.Var{Name: "commit protocol managed resource"}
}

func (c OnePhaseCommitProtocol) resourceGet(t *testcase.T) CRD {
	return c.resource().Get(t).(CRD)
}

func (c OnePhaseCommitProtocol) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c OnePhaseCommitProtocol) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c OnePhaseCommitProtocol) Spec(s *testcase.Spec) {
	s.HasSideEffect()
	factoryLet(s, c.FixtureFactory)

	s.Before(func(t *testcase.T) {
		manager, crd := c.Subject(t)
		c.manager().Set(t, manager)
		c.resource().Set(t, crd)
	})

	// clean ahead before testing suite
	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			DeleteAllEntity(t, c.resourceGet(t), c.Context(t))
		})
	})

	s.Around(func(t *testcase.T) func() {
		r := c.resourceGet(t)
		// early load the resource ensure proper cleanup
		return func() {
			DeleteAllEntity(t, r, c.Context(t))
		}
	})

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			ptr := CreatePTR(factoryGet(t), c.T)
			CreateEntity(t, c.resourceGet(t), tx, ptr)
			id := HasID(t, ptr)

			require.Nil(t, c.managerGet(t).CommitTx(tx))

			t.Log(`using the tx context after commit should yield error`)
			_, err = c.resourceGet(t).FindByID(tx, newT(c.T), id)
			require.Error(t, err)
			require.Error(t, c.resourceGet(t).Create(tx, CreatePTR(factoryGet(t), c.T)))
			require.Error(t, c.resourceGet(t).FindAll(tx).Err())

			if updater, ok := c.resourceGet(t).(frameless.Updater); ok {
				require.Error(t, updater.Update(tx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						c.resourceGet(t)))
			}

			require.Error(t, c.resourceGet(t).DeleteByID(tx, id))
			require.Error(t, c.resourceGet(t).DeleteAll(tx))

			Waiter.Wait()
		})
		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.Context(t)
			ctx, err := c.managerGet(t).BeginTx(ctx)
			require.Nil(t, err)
			ptr := CreatePTR(factoryGet(t), c.T)
			require.Nil(t, c.resourceGet(t).Create(ctx, ptr))
			id, _ := extid.Lookup(ptr)
			require.Nil(t, c.managerGet(t).RollbackTx(ctx))

			_, err = c.resourceGet(t).FindByID(ctx, newT(c.T), id)
			require.Error(t, err)
			require.Error(t, c.resourceGet(t).FindAll(ctx).Err())
			require.Error(t, c.resourceGet(t).Create(ctx, CreatePTR(factoryGet(t), c.T)))

			if updater, ok := c.resourceGet(t).(frameless.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						c.resourceGet(t)))
			}

			require.Error(t, c.resourceGet(t).DeleteByID(ctx, id))
			require.Error(t, c.resourceGet(t).DeleteAll(ctx))
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)

			entity := CreatePTR(factoryGet(t), c.T)
			CreateEntity(t, c.resourceGet(t), tx, entity)
			id := HasID(t, entity)

			IsFindable(t, c.T, c.resourceGet(t), tx, id)         // can be found in tx Context
			IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), id) // is absent from the global Context

			require.Nil(t, c.managerGet(t).CommitTx(tx)) // after the commit

			actually := IsFindable(t, c.T, c.resourceGet(t), c.Context(t), id)
			require.Equal(t, entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			entity := CreatePTR(factoryGet(t), c.T)
			//require.Nil(t, Spec.resourceGet(t).Create(tx, entity))
			CreateEntity(t, c.resourceGet(t), tx, entity)

			id := HasID(t, entity)
			IsFindable(t, c.T, c.resourceGet(t), tx, id)
			IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), id)

			require.Nil(t, c.managerGet(t).RollbackTx(tx))

			IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.Context(t)
			entity := CreatePTR(factoryGet(t), c.T)

			CreateEntity(t, c.resourceGet(t), ctx, entity)
			id := HasID(t, entity)
			t.Defer(c.resourceGet(t).DeleteByID, ctx, id)

			tx, err := c.managerGet(t).BeginTx(ctx)
			require.Nil(t, err)

			IsFindable(t, c.T, c.resourceGet(t), tx, id)
			require.Nil(t, c.resourceGet(t).DeleteByID(tx, id))
			IsAbsent(t, c.T, c.resourceGet(t), tx, id)

			// in global Context it is findable
			IsFindable(t, c.T, c.resourceGet(t), c.Context(t), id)

			require.Nil(t, c.managerGet(t).CommitTx(tx))
			IsAbsent(t, c.T, c.resourceGet(t), c.Context(t), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.Context(t)
			entity := CreatePTR(factoryGet(t), c.T)
			CreateEntity(t, c.resourceGet(t), ctx, entity)
			id := HasID(t, entity)

			tx, err := c.managerGet(t).BeginTx(ctx)
			require.Nil(t, err)
			IsFindable(t, c.T, c.resourceGet(t), tx, id)
			require.Nil(t, c.resourceGet(t).DeleteByID(tx, id))
			IsAbsent(t, c.T, c.resourceGet(t), tx, id)
			IsFindable(t, c.T, c.resourceGet(t), c.Context(t), id)
			require.Nil(t, c.managerGet(t).RollbackTx(tx))
			IsFindable(t, c.T, c.resourceGet(t), c.Context(t), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.Context(t)
			ctx, err := c.managerGet(t).BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, c.managerGet(t).CommitTx(ctx))
			require.Error(t, c.managerGet(t).CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.Context(t)
			ctx, err := c.managerGet(t).BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, c.managerGet(t).RollbackTx(ctx))
			require.Error(t, c.managerGet(t).RollbackTx(ctx))
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

			t.Defer(DeleteAllEntity, t, c.resourceGet(t), c.Context(t))

			var globalContext = c.Context(t)

			tx1, err := c.managerGet(t).BeginTx(globalContext)
			require.Nil(t, err)
			t.Log(`given tx1 is began`)

			e1 := CreatePTR(factoryGet(t), c.T)
			require.Nil(t, c.resourceGet(t).Create(tx1, e1))
			IsFindable(t, c.T, c.resourceGet(t), tx1, HasID(t, e1))
			IsAbsent(t, c.T, c.resourceGet(t), globalContext, HasID(t, e1))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := c.managerGet(t).BeginTx(tx1)
			require.Nil(t, err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := CreatePTR(factoryGet(t), c.T)
			require.Nil(t, c.resourceGet(t).Create(tx2InTx1, e2))
			IsFindable(t, c.T, c.resourceGet(t), tx2InTx1, HasID(t, e2))    // tx2 can see e2
			IsAbsent(t, c.T, c.resourceGet(t), globalContext, HasID(t, e2)) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent(t, c.T, c.resourceGet(t), globalContext, HasID(t, e1))
			IsAbsent(t, c.T, c.resourceGet(t), globalContext, HasID(t, e2))

			require.Nil(t, c.managerGet(t).CommitTx(tx2InTx1), `"inner" tx should be considered done`)
			require.Nil(t, c.managerGet(t).CommitTx(tx1), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable(t, c.T, c.resourceGet(t), globalContext, HasID(t, e1))
			IsFindable(t, c.T, c.resourceGet(t), globalContext, HasID(t, e2))
		})

		s.Describe(`Publisher`, c.specPublisher)
	})
}

func (c OnePhaseCommitProtocol) specPublisher(s *testcase.Spec) {
	s.Context(`Creator`, c.specCreatorPublisher)
	s.Context(`Updater`, c.specUpdaterPublisher)
	s.Context(`Deleter`, c.specDeleterPublisher)
}

func (c OnePhaseCommitProtocol) specCreatorPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) CreatorPublisherSubject {
		p, ok := c.resourceGet(t).(CreatorPublisherSubject)
		if !ok {
			t.Skipf(`%T doesn't supply frameless Publisher and Creator`, c.resourceGet(t))
		}
		return p
	}

	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		subscriberLet(s, "Create", func(event interface{}) bool {
			_, ok := event.(frameless.CreateEvent)
			return ok
		})
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			t.Defer(c.managerGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(factoryGet(t), c.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			return publisher(t).SubscribeToCreatorEvents(ctxGet(t), subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) frameless.Subscription {
			sub, err := subject(t)
			require.Nil(t, err)
			return sub
		}

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			require.NotNil(t, sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the storage`)
			for _, entity := range eventsGet(t) {
				CreateEntity(t, c.resourceGet(t), ctxGet(t), entity)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))

			var es []frameless.CreateEvent
			for _, ent := range eventsGet(t) {
				es = append(es, frameless.CreateEvent{Entity: base(ent)})
			}
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, es, subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (c OnePhaseCommitProtocol) specUpdaterPublisher(s *testcase.Spec) {
	updater := func(t *testcase.T) UpdaterSubject {
		u, ok := c.resourceGet(t).(UpdaterSubject)
		if !ok {
			t.Skipf(`%T doesn't supply resources.Updater`, c.resourceGet(t))
		}
		return u
	}

	updaterPublisher := func(t *testcase.T) UpdaterPublisherSubject {
		u, ok := c.resourceGet(t).(UpdaterPublisherSubject)
		if !ok {
			t.Skipf(`%T doesn't supply frameless Updater+Publisher`, c.resourceGet(t))
		}
		return u
	}

	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subscriberLet(s, `Update`, func(event interface{}) bool {
			_, ok := event.(frameless.UpdateEvent)
			return ok
		})
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			t.Defer(c.managerGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(factoryGet(t), c.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			return updaterPublisher(t).SubscribeToUpdaterEvents(ctxGet(t), subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) frameless.Subscription {
			sub, err := subject(t)
			require.Nil(t, err)
			return sub
		}

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			require.NotNil(t, sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`and then events created in the storage outside of the current transaction`)
			for _, ptr := range eventsGet(t) {
				CreateEntity(t, c.resourceGet(t), c.Context(t), ptr)
			}

			t.Log(`then events being updated`)
			for _, ptr := range eventsGet(t) {
				UpdateEntity(t, updater(t), ctxGet(t), ptr)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))

			var es []frameless.UpdateEvent
			for _, ent := range eventsGet(t) {
				es = append(es, frameless.UpdateEvent{Entity: base(ent)})
			}
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, es, subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (c OnePhaseCommitProtocol) specDeleterPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) DeleterPublisherSubject {
		u, ok := c.resourceGet(t).(DeleterPublisherSubject)
		if !ok {
			t.Skipf(`%T doesn't supply frameless Deleter+Publisher`, c.resourceGet(t))
		}
		return u
	}

	s.Describe(`#SubscribeToDeleteByID`, func(s *testcase.Spec) {
		subscriberLet(s, "DeleteByID", func(event interface{}) bool {
			_, ok := event.(frameless.DeleteByIDEvent)
			return ok
		})
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			t.Defer(c.managerGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return CreatePTR(factoryGet(t), c.T)
		})
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(ctxGet(t), subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) frameless.Subscription {
			sub, err := subject(t)
			require.Nil(t, err)
			return sub
		}

		hasDeleteEntity := DeleterPublisher{}.hasDeleteEntity

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			sub := onSuccess(t)
			require.NotNil(t, sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			t.Log(`given entity already created during the transaction`)
			CreateEntity(t, c.resourceGet(t), ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted during the transaction`)
			DeleteEntity(t, c.resourceGet(t), ctxGet(t), entity.Get(t))
		})

		s.Then(`before a commit, delete events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, delete events will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.False(tb, subscriberGet(t).EventsLen() < 1)
			})

			hasDeleteEntity(t, subscriberGet(t).Events, frameless.DeleteByIDEvent{ID: HasID(t, entity.Get(t))})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})
	})

	s.Describe(`#SubscribeToDeleteAll`, func(s *testcase.Spec) {
		subscriberLet(s, "DeleteAll", func(event interface{}) bool {
			_, ok := event.(frameless.DeleteAllEvent)
			return ok
		})
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := c.managerGet(t).BeginTx(c.Context(t))
			require.Nil(t, err)
			t.Defer(c.managerGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return CreatePTR(factoryGet(t), c.T)
		})
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			return publisher(t).SubscribeToDeleterEvents(ctxGet(t), subscriberGet(t))
		}

		s.Before(func(t *testcase.T) {
			subscriberGet(t).Name = `SubscribeToDeleteAll`
			t.Log(`given a subscription is made`)
			sub, err := subject(t)
			require.Nil(t, err)
			require.NotNil(t, sub)
			subscription.Set(t, sub)
			t.Defer(sub.Close)

			_ = entity
			t.Log(`given entity already created`)
			// TODO: why this makes a DeleteAll event somehow?
			CreateEntity(t, c.resourceGet(t), ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted`)
			DeleteAllEntity(t, c.resourceGet(t), ctxGet(t))
			Waiter.Wait()
		})

		s.Then(`before a commit, deleteAll events will be absent`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a commit, delete all event will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.True(tb, subscriberGet(t).EventsLen() == 1,
					`one event was expected, but didn't arrived`)
				require.Contains(tb, subscriberGet(t).Events(), frameless.DeleteAllEvent{})
			})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, c.managerGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}
