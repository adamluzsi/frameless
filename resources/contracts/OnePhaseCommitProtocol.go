package contracts

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type OnePhaseCommitProtocol struct {
	Subject interface {
		minimumRequirements
		resources.OnePhaseCommitProtocol
	}
	T              interface{}
	FixtureFactory FixtureFactory
}

func (spec OnePhaseCommitProtocol) Test(t *testing.T) {
	spec.spec(t)
}

func (spec OnePhaseCommitProtocol) Benchmark(b *testing.B) {
	spec.spec(b)
}

func (spec OnePhaseCommitProtocol) spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	defer s.Finish()
	s.HasSideEffect()

	// clean ahead before testing suite
	DeleteAllEntity(tb, spec.Subject, spec.Context(), spec.T)
	s.After(func(t *testcase.T) { DeleteAllEntity(tb, spec.Subject, spec.Context(), spec.T) })

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on context with finished tx`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			t.Defer(spec.Subject.DeleteByID, spec.Context(), spec.T, id)
			require.Nil(t, spec.Subject.CommitTx(ctx))

			_, err = spec.Subject.FindByID(ctx, spec.T, id)
			require.Error(t, err)
			require.Error(t, spec.Subject.Create(ctx, spec.FixtureFactory.Create(spec.T)))
			require.Error(t, spec.Subject.FindAll(ctx, spec.T).Err())

			if updater, ok := spec.Subject.(resources.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.Subject))
			}

			require.Error(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			require.Error(t, spec.Subject.DeleteAll(ctx, spec.T))
		})
		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on context with finished tx`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			require.Nil(t, spec.Subject.RollbackTx(ctx))

			_, err = spec.Subject.FindByID(ctx, spec.T, id)
			require.Error(t, err)
			require.Error(t, spec.Subject.FindAll(ctx, spec.T).Err())
			require.Error(t, spec.Subject.Create(ctx, spec.FixtureFactory.Create(spec.T)))

			if updater, ok := spec.Subject.(resources.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.Subject))
			}

			require.Error(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			require.Error(t, spec.Subject.DeleteAll(ctx, spec.T))
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id := HasID(t, entity)

			IsFindable(t, spec.Subject, ctx, newEntityFunc(spec.T), id)          // can be found in tx context
			IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id) // is absent from the global context

			require.Nil(t, spec.Subject.CommitTx(ctx)) // after the commit

			actually := IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
			require.Equal(t, entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))

			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)

			found, err := spec.Subject.FindByID(spec.Context(), newEntity(spec.T), id)
			require.Nil(t, err)
			require.False(t, found)

			require.Nil(t, spec.Subject.RollbackTx(ctx))

			found, err = spec.Subject.FindByID(spec.Context(), newEntity(spec.T), id)
			require.Nil(t, err)
			require.False(t, found)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id := HasID(t, entity)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)

			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			IsFindable(t, spec.Subject, ctx, newEntityFunc(spec.T), id)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			IsAbsent(t, spec.Subject, ctx, newEntityFunc(spec.T), id)

			// in global context it is findable
			IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)

			require.Nil(t, spec.Subject.CommitTx(ctx))
			IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			IsFindable(t, spec.Subject, ctx, newEntityFunc(spec.T), id)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			IsAbsent(t, spec.Subject, ctx, newEntityFunc(spec.T), id)
			IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
			require.Nil(t, spec.Subject.RollbackTx(ctx))
			IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.CommitTx(ctx))
			require.Error(t, spec.Subject.CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.RollbackTx(ctx))
			require.Error(t, spec.Subject.RollbackTx(ctx))
		})

		s.Test(`BeginTx should be callable multiple times to ensure emulate multi level transaction`, func(t *testcase.T) {
			t.Log(
				`Even if the current driver or resource don't support multi level transactions`,
				`It should still accept multiple transaction begin for a given context`,
				`The benefit of this is that low level components that needs to ensure transactional execution,`,
				`they should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor itself.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			t.Defer(DeleteAllEntity, t, spec.Subject, spec.Context(), spec.T)

			var globalContext = spec.Context()

			tx1, err := spec.Subject.BeginTx(globalContext)
			require.Nil(t, err)
			e1 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(tx1, e1))
			IsFindable(t, spec.Subject, tx1, newEntityFunc(spec.T), HasID(t, e1))

			tx2InTx1, err := spec.Subject.BeginTx(globalContext)
			require.Nil(t, err)

			e2 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(tx2InTx1, e2))
			IsFindable(t, spec.Subject, tx2InTx1, newEntityFunc(spec.T), HasID(t, e2)) // tx2 entity should be visible
			IsFindable(t, spec.Subject, tx1, newEntityFunc(spec.T), HasID(t, e1))      // so the entity made in tx1

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e1))
			IsAbsent(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e2))

			require.Nil(t, spec.Subject.CommitTx(tx2InTx1), `"inner" tx should be considered done`)
			require.Nil(t, spec.Subject.CommitTx(tx1), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e1))
			IsFindable(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e2))
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
	creatorPublisher, ok := spec.Subject.(resources.CreatorPublisher)
	if !ok {
		return
	}

	s.Describe(`#SubscribeToCreate`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.Subject.BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.Subject.RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(spec.FixtureFactory, spec.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return creatorPublisher.SubscribeToCreate(ctxGet(t), spec.T, subscriberGet(t))
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
				CreateEntity(t, spec.Subject, ctxGet(t), entity)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))

			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, toBaseValues(eventsGet(t)), subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (spec OnePhaseCommitProtocol) specUpdaterPublisher(s *testcase.Spec) {
	updater, ok := spec.Subject.(interface {
		minimumRequirements
		resources.Updater
	})
	if !ok {
		return
	}

	updaterPublisher, ok := spec.Subject.(resources.UpdaterPublisher)
	if !ok {
		return
	}

	s.Describe(`#SubscribeToUpdate`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.Subject.BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.Subject.RollbackTx, ctxInTx)
			return ctxInTx
		})
		events := s.Let(`events`, func(t *testcase.T) interface{} {
			return genEntities(spec.FixtureFactory, spec.T)
		})
		eventsGet := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return updaterPublisher.SubscribeToUpdate(ctxGet(t), spec.T, subscriberGet(t))
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
				CreateEntity(t, spec.Subject, spec.FixtureFactory.Context(), ptr)
			}

			t.Log(`then events being updated`)
			for _, ptr := range eventsGet(t) {
				UpdateEntity(t, updater, ctxGet(t), ptr)
			}
		})

		s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, events will be present`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))

			AsyncTester.Assert(t, func(tb testing.TB) {
				require.ElementsMatch(tb, toBaseValues(eventsGet(t)), subscriberGet(t).Events())
			})
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}

func (spec OnePhaseCommitProtocol) specDeleterPublisher(s *testcase.Spec) {
	publisher, ok := spec.Subject.(resources.DeleterPublisher)
	if !ok {
		return
	}

	s.Describe(`#SubscribeToDeleteByID`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.Subject.BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.Subject.RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return publisher.SubscribeToDeleteByID(ctxGet(t), spec.T, subscriberGet(t))
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
			CreateEntity(t, spec.Subject, ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted during the transaction`)
			DeleteEntity(t, spec.Subject, ctxGet(t), entity.Get(t))
		})

		s.Then(`before a commit, delete events will be absent`, func(t *testcase.T) {
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))
		})

		s.Then(`after a commit, delete events will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.False(tb, subscriberGet(t).EventsLen() < 1)
			})
			hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})
	})

	s.Describe(`#SubscribeToDeleteAll`, func(s *testcase.Spec) {
		subscriber.Let(s, nil)
		subscription.Let(s, nil)
		ctx.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given we are in transaction`)
			ctxInTx, err := spec.Subject.BeginTx(spec.FixtureFactory.Context())
			require.Nil(t, err)
			t.Defer(spec.Subject.RollbackTx, ctxInTx)
			return ctxInTx
		})
		entity := s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.T)
		})
		subject := func(t *testcase.T) (resources.Subscription, error) {
			return publisher.SubscribeToDeleteAll(ctxGet(t), spec.T, subscriberGet(t))
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
			CreateEntity(t, spec.Subject, ctxGet(t), entity.Get(t))

			t.Log(`and then the entity is also deleted`)
			DeleteAllEntity(t, spec.Subject, ctxGet(t), spec.T)
			Waiter.Wait()
		})

		s.Then(`before a commit, deleteAll events will be absent`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a commit, delete all event will arrive to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.CommitTx(ctxGet(t)))
			AsyncTester.Assert(t, func(tb testing.TB) {
				require.True(tb, subscriberGet(t).EventsLen() == 1,
					`one event was expected, but didn't arrived`)
			})
		})

		s.Then(`after a rollback, there won't be any delete events sent to the subscriber`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.RollbackTx(ctxGet(t)))
			require.Empty(t, subscriberGet(t).Events())
		})

		s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
			require.Nil(t, spec.Subject.RollbackTx(ctxGet(t)))
			Waiter.Wait()
			require.Empty(t, subscriberGet(t).Events())
		})
	})
}
