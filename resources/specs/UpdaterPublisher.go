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
		const contextKey = `getContext`
		const subscriberKey = `subscriber`
		const subscriptionKey = `subscription`
		getSubscriber := func(t *testcase.T, key string) *eventSubscriber {
			return t.I(key).(*eventSubscriber)
		}
		subscriber := func(t *testcase.T) *eventSubscriber {
			return getSubscriber(t, subscriberKey)
		}
		subject := func(t *testcase.T) (resources.Subscription, error) {
			subscription, err := spec.Subject.SubscribeToUpdate(spec.EntityType, subscriber(t))
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

		getContext := func(t *testcase.T) context.Context {
			return t.I(contextKey).(context.Context)
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
			t.Defer(spec.Subject.DeleteByID, getContext(t), spec.EntityType, id)
			t.Let(entityKey, entityPtr)

			t.Log(`given a subscription is made`)
			onSuccess(t)
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriber(t).events)
		})

		s.And(`update event made`, func(s *testcase.Spec) {
			const updatedEntityKey = `updated-entity`
			s.Before(func(t *testcase.T) {
				id, _ := resources.LookupID(t.I(entityKey))
				updatedEntityPtr := spec.createEntity()
				require.Nil(t, resources.SetID(updatedEntityPtr, id))
				require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
				t.Let(updatedEntityKey, toBaseValue(updatedEntityPtr))
			})

			s.Then(`subscriber receive the event`, func(t *testcase.T) {
				require.Contains(t, subscriber(t).events, t.I(updatedEntityKey))
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
					})

					s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
						require.Len(t, subscriber(t).events, 1)
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
					sub, err := spec.Subject.SubscribeToUpdate(spec.EntityType, othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					require.Contains(t, subscriber(t).events, t.I(updatedEntityKey))
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					require.Empty(t, othSubscriber(t).events)
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					const furtherEventUpdateKey = `further event update`
					s.Before(func(t *testcase.T) {
						id, _ := resources.LookupID(t.I(entityKey))
						updatedEntityPtr := spec.createEntity()
						require.Nil(t, resources.SetID(updatedEntityPtr, id))
						require.Nil(t, spec.Subject.Update(getContext(t), updatedEntityPtr))
						t.Let(furtherEventUpdateKey, toBaseValue(updatedEntityPtr))
					})

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						require.Contains(t, subscriber(t).events, t.I(updatedEntityKey), `missing old update events`)
						require.Contains(t, subscriber(t).events, t.I(furtherEventUpdateKey), `missing new update events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						require.NotContains(t, othSubscriber(t).events, t.I(updatedEntityKey))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						require.Contains(t, othSubscriber(t).events, t.I(furtherEventUpdateKey))
					})
				})
			})

			if res, ok := spec.Subject.(resources.OnePhaseCommitProtocol); ok {
				s.Describe(`relationship with OnePhaseCommitProtocol`, func(s *testcase.Spec) {
					s.Let(contextKey, func(t *testcase.T) interface{} {
						t.Log(`given we are in transaction`)
						ctxInTx, err := res.BeginTx(spec.context())
						require.Nil(t, err)
						t.Defer(res.RollbackTx, ctxInTx)
						return ctxInTx
					})

					s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
						require.Empty(t, subscriber(t).events)
						require.Nil(t, res.CommitTx(getContext(t)))
					})

					s.Then(`after a commit, events will be present`, func(t *testcase.T) {
						require.Nil(t, res.CommitTx(getContext(t)))
						require.Contains(t, subscriber(t).events, t.I(updatedEntityKey))
					})

					s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
						require.Nil(t, res.RollbackTx(getContext(t)))
						require.Empty(t, subscriber(t).events)
					})
				})
			}
		})
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
