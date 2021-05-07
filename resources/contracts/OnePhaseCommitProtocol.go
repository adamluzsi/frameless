package contracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type OnePhaseCommitProtocol struct {
	T
	Subject        func(testing.TB) OnePhaseCommitProtocolSubject
	FixtureFactory FixtureFactory
}

type OnePhaseCommitProtocolSubject interface {
	CRD
	resources.OnePhaseCommitProtocol
}

func (spec OnePhaseCommitProtocol) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return spec.Subject(t)
		},
	}
}

func (spec OnePhaseCommitProtocol) resourceGet(t *testcase.T) OnePhaseCommitProtocolSubject {
	return spec.resource().Get(t).(OnePhaseCommitProtocolSubject)
}

func (spec OnePhaseCommitProtocol) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec OnePhaseCommitProtocol) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec OnePhaseCommitProtocol) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	defer s.Finish()
	s.HasSideEffect()

	spec.resource().Let(s, nil)

	// clean ahead before testing suite
	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			DeleteAllEntity(t, spec.resourceGet(t), spec.Context(), spec.T)
		})
	})

	s.Around(func(t *testcase.T) func() {
		r := spec.resourceGet(t)
		// early load the resource ensure proper cleanup
		return func() {
			DeleteAllEntity(t, r, spec.Context(), spec.T)
		}
	})

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := spec.resourceGet(t).BeginTx(spec.Context())
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			CreateEntity(t, spec.resourceGet(t), tx, ptr)
			id := HasID(t, ptr)

			require.Nil(t, spec.resourceGet(t).CommitTx(tx))

			t.Log(`using the tx context after commit should yield error`)
			_, err = spec.resourceGet(t).FindByID(tx, spec.T, id)
			require.Error(t, err)
			require.Error(t, spec.resourceGet(t).Create(tx, spec.FixtureFactory.Create(spec.T)))
			require.Error(t, spec.resourceGet(t).FindAll(tx, spec.T).Err())

			if updater, ok := spec.resourceGet(t).(resources.Updater); ok {
				require.Error(t, updater.Update(tx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.resourceGet(t)))
			}

			require.Error(t, spec.resourceGet(t).DeleteByID(tx, spec.T, id))
			require.Error(t, spec.resourceGet(t).DeleteAll(tx, spec.T))

			Waiter.Wait()
		})
		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.resourceGet(t).BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.resourceGet(t).Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctx))

			_, err = spec.resourceGet(t).FindByID(ctx, newEntity(spec.T), id)
			require.Error(t, err)
			require.Error(t, spec.resourceGet(t).FindAll(ctx, spec.T).Err())
			require.Error(t, spec.resourceGet(t).Create(ctx, spec.FixtureFactory.Create(spec.T)))

			if updater, ok := spec.resourceGet(t).(resources.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.resourceGet(t)))
			}

			require.Error(t, spec.resourceGet(t).DeleteByID(ctx, spec.T, id))
			require.Error(t, spec.resourceGet(t).DeleteAll(ctx, spec.T))
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := spec.resourceGet(t).BeginTx(spec.Context())
			require.Nil(t, err)

			entity := spec.FixtureFactory.Create(spec.T)
			CreateEntity(t, spec.resourceGet(t), tx, entity)
			id := HasID(t, entity)

			IsFindable(t, spec.T, spec.resourceGet(t), tx, id)           // can be found in tx Context
			IsAbsent(t, spec.T, spec.resourceGet(t), spec.Context(), id) // is absent from the global Context

			require.Nil(t, spec.resourceGet(t).CommitTx(tx)) // after the commit

			actually := IsFindable(t, spec.T, spec.resourceGet(t), spec.Context(), id)
			require.Equal(t, entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := spec.resourceGet(t).BeginTx(spec.Context())
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.T)
			//require.Nil(t, Spec.resourceGet(t).Create(tx, entity))
			CreateEntity(t, spec.resourceGet(t), tx, entity)

			id := HasID(t, entity)
			IsFindable(t, spec.T, spec.resourceGet(t), tx, id)
			IsAbsent(t, spec.T, spec.resourceGet(t), spec.Context(), id)

			require.Nil(t, spec.resourceGet(t).RollbackTx(tx))

			IsAbsent(t, spec.T, spec.resourceGet(t), spec.Context(), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)

			CreateEntity(t, spec.resourceGet(t), ctx, entity)
			id := HasID(t, entity)
			t.Defer(spec.resourceGet(t).DeleteByID, ctx, spec.T, id)

			tx, err := spec.resourceGet(t).BeginTx(ctx)
			require.Nil(t, err)

			IsFindable(t, spec.T, spec.resourceGet(t), tx, id)
			require.Nil(t, spec.resourceGet(t).DeleteByID(tx, spec.T, id))
			IsAbsent(t, spec.T, spec.resourceGet(t), tx, id)

			// in global Context it is findable
			IsFindable(t, spec.T, spec.resourceGet(t), spec.Context(), id)

			require.Nil(t, spec.resourceGet(t).CommitTx(tx))
			IsAbsent(t, spec.T, spec.resourceGet(t), spec.Context(), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			CreateEntity(t, spec.resourceGet(t), ctx, entity)
			id := HasID(t, entity)

			tx, err := spec.resourceGet(t).BeginTx(ctx)
			require.Nil(t, err)
			IsFindable(t, spec.T, spec.resourceGet(t), tx, id)
			require.Nil(t, spec.resourceGet(t).DeleteByID(tx, spec.T, id))
			IsAbsent(t, spec.T, spec.resourceGet(t), tx, id)
			IsFindable(t, spec.T, spec.resourceGet(t), spec.Context(), id)
			require.Nil(t, spec.resourceGet(t).RollbackTx(tx))
			IsFindable(t, spec.T, spec.resourceGet(t), spec.Context(), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.resourceGet(t).BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.resourceGet(t).CommitTx(ctx))
			require.Error(t, spec.resourceGet(t).CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.resourceGet(t).BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctx))
			require.Error(t, spec.resourceGet(t).RollbackTx(ctx))
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

			t.Defer(DeleteAllEntity, t, spec.resourceGet(t), spec.Context(), spec.T)

			var globalContext = spec.Context()

			tx1, err := spec.resourceGet(t).BeginTx(globalContext)
			require.Nil(t, err)
			e1 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.resourceGet(t).Create(tx1, e1))
			IsFindable(t, spec.T, spec.resourceGet(t), tx1, HasID(t, e1))

			tx2InTx1, err := spec.resourceGet(t).BeginTx(globalContext)
			require.Nil(t, err)

			e2 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.resourceGet(t).Create(tx2InTx1, e2))
			IsFindable(t, spec.T, spec.resourceGet(t), tx2InTx1, HasID(t, e2)) // tx2 entity should be visible
			IsFindable(t, spec.T, spec.resourceGet(t), tx1, HasID(t, e1))      // so the entity made in tx1

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent(t, spec.T, spec.resourceGet(t), globalContext, HasID(t, e1))
			IsAbsent(t, spec.T, spec.resourceGet(t), globalContext, HasID(t, e2))

			require.Nil(t, spec.resourceGet(t).CommitTx(tx2InTx1), `"inner" tx should be considered done`)
			require.Nil(t, spec.resourceGet(t).CommitTx(tx1), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable(t, spec.T, spec.resourceGet(t), globalContext, HasID(t, e1))
			IsFindable(t, spec.T, spec.resourceGet(t), globalContext, HasID(t, e2))
		})

		s.Describe(`CreatorPublisher`, spec.specCreatorPublisher)
		s.Describe(`UpdaterPublisher`, spec.specUpdaterPublisher)
		s.Describe(`DeleterPublisher`, spec.specDeleterPublisher)
	})
}

func (spec OnePhaseCommitProtocol) Context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec OnePhaseCommitProtocol) specCreatorPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) resources.CreatorPublisher {
		p, ok := spec.resourceGet(t).(resources.CreatorPublisher)
		if !ok {
			t.Skipf(`%T doesn't supply resources.CreatorPublisher`, spec.resourceGet(t))
		}
		return p
	}

	s.Describe(`#SubscribeToCreate`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.resourceGet(t).BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.resourceGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(spec.FixtureFactory, spec.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return publisher(t).SubscribeToCreate(ctxGet(t), spec.T, subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) resources.Subscription {
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
				CreateEntity(t, spec.resourceGet(t), ctxGet(t), entity)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))

			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, toBaseValues(eventsGet(t)), subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (spec OnePhaseCommitProtocol) specUpdaterPublisher(s *testcase.Spec) {
	updater := func(t *testcase.T) crud {
		u, ok := spec.resourceGet(t).(crud)
		if !ok {
			t.Skipf(`%T doesn't supply resources.Updater`, spec.resourceGet(t))
		}
		return u
	}

	updaterPublisher := func(t *testcase.T) resources.UpdaterPublisher {
		u, ok := spec.resourceGet(t).(resources.UpdaterPublisher)
		if !ok {
			t.Skipf(`%T doesn't supply resources.UpdaterPublisher`, spec.resourceGet(t))
		}
		return u
	}

	s.Describe(`#SubscribeToUpdate`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.resourceGet(t).BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.resourceGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(spec.FixtureFactory, spec.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return updaterPublisher(t).SubscribeToUpdate(ctxGet(t), spec.T, subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) resources.Subscription {
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
				CreateEntity(t, spec.resourceGet(t), spec.FixtureFactory.Context(), ptr)
			}

			t.Log(`then events being updated`)
			for _, ptr := range eventsGet(t) {
				UpdateEntity(t, updater(t), ctxGet(t), ptr)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))

			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, toBaseValues(eventsGet(t)), subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (spec OnePhaseCommitProtocol) specDeleterPublisher(s *testcase.Spec) {
	publisher := func(t *testcase.T) resources.DeleterPublisher {
		u, ok := spec.resourceGet(t).(resources.DeleterPublisher)
		if !ok {
			t.Skipf(`%T doesn't supply resources.DeleterPublisher`, spec.resourceGet(t))
		}
		return u
	}

	s.Describe(`#SubscribeToDeleteByID`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.resourceGet(t).BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.resourceGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return publisher(t).SubscribeToDeleteByID(ctxGet(t), spec.T, subscriberGet(t))
		}
		onSuccess := func(t *testcase.T) resources.Subscription {
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
			CreateEntity(t, spec.resourceGet(t), ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted during the transaction`)
			DeleteEntity(t, spec.resourceGet(t), ctxGet(t), entity.Get(t))
		})

		s.Then(`before a commit, delete events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, delete events will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.False(tb, subscriberGet(t).EventsLen() < 1)
			})
			hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})
	})

	s.Describe(`#SubscribeToDeleteAll`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.resourceGet(t).BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.resourceGet(t).RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return publisher(t).SubscribeToDeleteAll(ctxGet(t), spec.T, subscriberGet(t))
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
			CreateEntity(t, spec.resourceGet(t), ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted`)
			DeleteAllEntity(t, spec.resourceGet(t), ctxGet(t), spec.T)
			Waiter.Wait()
		})

		s.Then(`before a commit, deleteAll events will be absent`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a commit, delete all event will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.True(tb, subscriberGet(t).EventsLen() == 1,
					`one event was expected, but didn't arrived`)
			})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.resourceGet(t).RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}
