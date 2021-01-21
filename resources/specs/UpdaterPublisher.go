package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type UpdaterPublisher struct {
	Subject interface {
		minimumRequirements
		resources.Updater
		resources.UpdaterPublisher
	}
	T              interface{}
	FixtureFactory FixtureFactory
}

func (spec UpdaterPublisher) Test(t *testing.T) {
	t.Run(`UpdaterPublisher`, func(t *testing.T) {
		spec.Spec(testcase.NewSpec(t))
	})
}

func (spec UpdaterPublisher) Benchmark(b *testing.B) {
	b.Run(`UpdaterPublisher`, func(b *testing.B) {
		spec.Spec(testcase.NewSpec(b))
	})
}

func (spec UpdaterPublisher) Spec(s *testcase.Spec) {
	s.Describe(`#SubscribeToUpdate`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) (resources.Subscription, error) {
			subscription, err := spec.Subject.SubscribeToUpdate(ctxGet(t), spec.T, subscriberGet(t))
			if err == nil && subscription != nil {
				t.Let(subscriptionKey, subscription)
				t.Defer(subscription.Close)
			}
			return subscription, err
		}
		onSuccess := func(t *testcase.T) {
			sub, err := subject(t)
			require.Nil(t, err)
			require.NotNil(t, sub)
		}

		ctx.Let(s, func(t *testcase.T) interface{} {
			return spec.context()
		})

		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t)
		})

		const entityKey = `entity`
		entity := s.Let(entityKey, func(t *testcase.T) interface{} {
			ptr := spec.createEntity()
			CreateEntity(t, spec.Subject, ctxGet(t), ptr)
			return ptr
		}).EagerLoading(s)
		getID := func(t *testcase.T) interface{} {
			id, _ := resources.LookupID(entity.Get(t))
			return id
		}

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			onSuccess(t)
		})

		s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.And(`update event made`, func(s *testcase.Spec) {
			const updatedEntityKey = `updated-entity`
			updatedEntity := s.Let(updatedEntityKey, func(t *testcase.T) interface{} {
				entityWithNewValuesPtr := spec.createEntity()
				require.Nil(t, resources.SetID(entityWithNewValuesPtr, getID(t)))
				UpdateEntity(t, spec.Subject, ctxGet(t), entityWithNewValuesPtr)
				Waiter.While(func() bool { return subscriberGet(t).EventsLen() < 1 })
				return toBaseValue(entityWithNewValuesPtr)
			}).EagerLoading(s)

			s.Then(`subscriberGet receive the event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), updatedEntity.Get(t))
			})

			s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, t.I(subscriptionKey).(resources.Subscription).Close())
				})

				s.And(`more events made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						id, _ := resources.LookupID(t.I(entityKey))
						updatedEntityPtr := spec.createEntity()
						require.Nil(t, resources.SetID(updatedEntityPtr, id))
						require.Nil(t, spec.Subject.Update(ctxGet(t), updatedEntityPtr))
						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < 1
						})
					})

					s.Then(`subscriberGet no longer receive them`, func(t *testcase.T) {
						require.Len(t, subscriberGet(t).Events(), 1)
					})
				})
			})

			s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
				const othSubscriberKey = `oth-subscriberGet`
				othSubscriber := func(t *testcase.T) *eventSubscriber {
					return getSubscriber(t, othSubscriberKey)
				}
				s.Before(func(t *testcase.T) {
					othSubscriber := newEventSubscriber(t)
					t.Let(othSubscriberKey, othSubscriber)
					sub, err := spec.Subject.SubscribeToUpdate(ctxGet(t), spec.T, othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriberGet still receive old events`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), updatedEntity.Get(t))
				})

				s.Then(`new subscriberGet do not receive old events`, func(t *testcase.T) {
					Waiter.Wait()
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					furtherEventUpdate := s.Let(`further event update`, func(t *testcase.T) interface{} {
						updatedEntityPtr := spec.createEntity()
						require.Nil(t, resources.SetID(updatedEntityPtr, getID(t)))
						UpdateEntity(t, spec.Subject, ctxGet(t), updatedEntityPtr)
						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < 2
						})
						Waiter.While(func() bool {
							return getSubscriber(t, othSubscriberKey).EventsLen() < 1
						})
						return toBaseValue(updatedEntityPtr)
					}).EagerLoading(s)

					s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
						require.Contains(t, subscriberGet(t).Events(), updatedEntity.Get(t), `missing old update events`)
						require.Contains(t, subscriberGet(t).Events(), furtherEventUpdate.Get(t), `missing new update events`)
					})

					s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
						Waiter.Wait()
						require.NotContains(t, othSubscriber(t).Events(), updatedEntity.Get(t))
					})

					s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
						require.Contains(t, othSubscriber(t).Events(), furtherEventUpdate.Get(t))
					})
				})
			})
		})

		s.Describe(`relationship with OnePhaseCommitProtocol`, spec.specOnePhaseCommitProtocol)

	})
}

func (spec UpdaterPublisher) specOnePhaseCommitProtocol(s *testcase.Spec) {
	res, ok := spec.Subject.(resources.OnePhaseCommitProtocol)
	if !ok {
		return
	}

	const entityKey = `entity`

	//TODO: fix, remove implicit reference to outer layer value definition
	entity := testcase.Var{Name: entityKey}

	updatedEntity := s.Let(`updated-entity`, func(t *testcase.T) interface{} {
		id, _ := resources.LookupID(entity.Get(t))
		updatedEntityPtr := spec.createEntity()
		require.Nil(t, resources.SetID(updatedEntityPtr, id))
		require.Nil(t, spec.Subject.Update(ctxGet(t), updatedEntityPtr))
		HasEntity(t, spec.Subject, ctxGet(t), updatedEntityPtr)
		return updatedEntityPtr
	}).EagerLoading(s)

	ctx.Let(s, func(t *testcase.T) interface{} {
		t.Log(`given we are in transaction`)
		ctxInTx, err := res.BeginTx(spec.context())
		require.Nil(t, err)
		t.Defer(res.RollbackTx, ctxInTx)
		return ctxInTx
	})

	s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
		Waiter.Wait()
		require.Empty(t, subscriberGet(t).Events())
		require.Nil(t, res.CommitTx(ctxGet(t)))
	})

	s.Then(`after a commit, events will be present`, func(t *testcase.T) {
		require.Nil(t, res.CommitTx(ctxGet(t)))
		AsyncTester.Assert(t, func(tb testing.TB) {
			require.False(tb, subscriberGet(t).EventsLen() < 1)
			require.Contains(tb, subscriberGet(t).Events(), toBaseValue(updatedEntity.Get(t)))
		})
	})

	s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
		require.Nil(t, res.RollbackTx(ctxGet(t)))
		Waiter.Wait()
		require.Empty(t, subscriberGet(t).Events())
	})
}

func (spec UpdaterPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec UpdaterPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.T)
}

func (spec UpdaterPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
