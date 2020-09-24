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
	EntityType     interface{}
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
			subscription, err := spec.Subject.SubscribeToUpdate(getContext(t), spec.EntityType, subscriber(t))
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

		s.Let(contextKey, func(t *testcase.T) interface{} {
			return spec.context()
		})

		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t)
		})

		const entityKey = `entity`
		s.Before(func(t *testcase.T) {
			t.Log(`given an entity is already stored`)
			entityPtr := spec.createEntity()
			require.Nil(t, spec.Subject.Create(getContext(t), entityPtr))
			id, _ := resources.LookupID(entityPtr)
			t.Defer(spec.Subject.DeleteByID, spec.context(), spec.EntityType, id)
			t.Let(entityKey, entityPtr)

			t.Log(`given a subscription is made`)
			onSuccess(t)
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriber(t).Events())
		})

		s.And(`update event made`, func(s *testcase.Spec) {
			const updatedEntityKey = `updated-entity`
			s.Before(func(t *testcase.T) {
				id, _ := resources.LookupID(t.I(entityKey))
				updatedEntityPtr := spec.createEntity()
				require.Nil(t, resources.SetID(updatedEntityPtr, id))
				require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
				t.Let(updatedEntityKey, toBaseValue(updatedEntityPtr))
				waitForLen(subscriber(t).EventsLen, 1)
			})

			s.Then(`subscriber receive the event`, func(t *testcase.T) {
				require.Contains(t, subscriber(t).Events(), t.I(updatedEntityKey))
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
						require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
						waitForLen(subscriber(t).EventsLen, 1)
					})

					s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
						require.Len(t, subscriber(t).Events(), 1)
					})
				})
			})

			s.And(`then new subscriber registered`, func(s *testcase.Spec) {
				const othSubscriberKey = `oth-subscriber`
				othSubscriber := func(t *testcase.T) *eventSubscriber {
					return getSubscriber(t, othSubscriberKey)
				}
				s.Before(func(t *testcase.T) {
					othSubscriber := newEventSubscriber(t)
					t.Let(othSubscriberKey, othSubscriber)
					sub, err := spec.Subject.SubscribeToUpdate(getContext(t), spec.EntityType, othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					require.Contains(t, subscriber(t).Events(), t.I(updatedEntityKey))
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					wait()
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					const furtherEventUpdateKey = `further event update`
					s.Before(func(t *testcase.T) {
						id, _ := resources.LookupID(t.I(entityKey))
						updatedEntityPtr := spec.createEntity()
						require.Nil(t, resources.SetID(updatedEntityPtr, id))
						require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
						t.Let(furtherEventUpdateKey, toBaseValue(updatedEntityPtr))
						waitForLen(subscriber(t).EventsLen, 2)
						waitForLen(getSubscriber(t, othSubscriberKey).EventsLen, 1)
					})

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						require.Contains(t, subscriber(t).Events(), t.I(updatedEntityKey), `missing old update events`)
						require.Contains(t, subscriber(t).Events(), t.I(furtherEventUpdateKey), `missing new update events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						wait()
						require.NotContains(t, othSubscriber(t).Events(), t.I(updatedEntityKey))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						require.Contains(t, othSubscriber(t).Events(), t.I(furtherEventUpdateKey))
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
	const updatedEntityKey = `updated-entity`

	s.Before(func(t *testcase.T) {
		id, _ := resources.LookupID(t.I(entityKey))
		updatedEntityPtr := spec.createEntity()
		require.Nil(t, resources.SetID(updatedEntityPtr, id))
		require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
		t.Let(updatedEntityKey, toBaseValue(updatedEntityPtr))
	})

	s.Let(contextKey, func(t *testcase.T) interface{} {
		t.Log(`given we are in transaction`)
		ctxInTx, err := res.BeginTx(spec.context())
		require.Nil(t, err)
		t.Defer(res.RollbackTx, ctxInTx)
		return ctxInTx
	})

	s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
		wait()
		require.Empty(t, subscriber(t).Events())
		require.Nil(t, res.CommitTx(getContext(t)))
	})

	s.Then(`after a commit, events will be present`, func(t *testcase.T) {
		require.Nil(t, res.CommitTx(getContext(t)))
		waitForLen(subscriber(t).EventsLen, 1)
		require.Contains(t, subscriber(t).Events(), t.I(updatedEntityKey))
	})

	s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
		require.Nil(t, res.RollbackTx(getContext(t)))
		wait()
		require.Empty(t, subscriber(t).Events())
	})
}

func (spec UpdaterPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec UpdaterPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.EntityType)
}

func (spec UpdaterPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
