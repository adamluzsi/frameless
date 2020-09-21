package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type CreatorPublisher struct {
	Subject interface {
		minimumRequirements
		resources.CreatorPublisher
	}
	EntityType     interface{}
	FixtureFactory FixtureFactory
}

func (spec CreatorPublisher) Test(t *testing.T) {
	t.Run(`CreatorPublisher`, func(t *testing.T) {
		spec.Spec(testcase.NewSpec(t))
	})
}

func (spec CreatorPublisher) Benchmark(b *testing.B) {
	b.Run(`CreatorPublisher`, func(b *testing.B) {
		spec.Spec(testcase.NewSpec(b))
	})
}

func (spec CreatorPublisher) Spec(s *testcase.Spec) {
	s.Describe(`#SubscribeToCreate`, func(s *testcase.Spec) {
		const subscriberKey = `subscriber`
		const subscriptionKey = `subscription`
		const contextKey = `getContext`
		getContext := func(t *testcase.T) context.Context {
			return t.I(contextKey).(context.Context)
		}
		getSubscriber := func(t *testcase.T, key string) *eventSubscriber {
			return t.I(key).(*eventSubscriber)
		}
		subscriber := func(t *testcase.T) *eventSubscriber {
			return getSubscriber(t, subscriberKey)
		}
		subject := func(t *testcase.T) (resources.Subscription, error) {
			subscription, err := spec.Subject.SubscribeToCreate(getContext(t), spec.EntityType, subscriber(t))
			if err == nil && subscription != nil {
				t.Let(subscriptionKey, subscription)
				t.Defer(subscription.Close)
			}
			return subscription, err
		}
		onSuccess := func(t *testcase.T) resources.Subscription {
			subscription, err := subject(t)
			require.Nil(t, err)
			return subscription
		}

		s.Let(contextKey, func(t *testcase.T) interface{} {
			return spec.context()
		})

		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t)
		})

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			require.NotNil(t, onSuccess(t))
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriber(t).events)
		})

		s.And(`events made`, func(s *testcase.Spec) {
			const eventsKey = `events`
			s.Before(func(t *testcase.T) {
				entities := spec.createEntities()
				for _, entity := range entities {
					require.Nil(t, spec.Subject.Create(getContext(t), entity))
					id, _ := resources.LookupID(entity)
					// we use a new context here to enforce that the cleaning will be done outside of any context.
					// It might fail but will ensure proper cleanup.
					t.Defer(spec.Subject.DeleteByID, spec.context(), spec.EntityType, id)
				}
				t.Let(eventsKey, toBaseValues(entities))
			})

			s.Then(`subscriber receive those events`, func(t *testcase.T) {
				require.ElementsMatch(t, t.I(eventsKey).([]interface{}), subscriber(t).events)
			})

			s.And(`subscription is cancelled by close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					sub := t.I(subscriptionKey).(resources.Subscription)
					require.Nil(t, sub.Close())
				})

				s.And(`more events made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						entities := spec.createEntities()
						for _, entity := range entities {
							require.Nil(t, spec.Subject.Create(getContext(t), entity))
							id, _ := resources.LookupID(entity)
							t.Defer(spec.Subject.DeleteByID, getContext(t), spec.EntityType, id)
						}
					})

					s.Then(`handler don't receive the new events`, func(t *testcase.T) {
						require.ElementsMatch(t, t.I(eventsKey).([]interface{}), subscriber(t).events)
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
					newSubscription, err := spec.Subject.SubscribeToCreate(getContext(t), spec.EntityType, othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, newSubscription)
					t.Defer(newSubscription.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					require.ElementsMatch(t, subscriber(t).events, t.I(eventsKey).([]interface{}))
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					t.Log(`new subscriber don't have the vents since it subscribed after events had been already fired`)
					require.Empty(t, othSubscriber(t).events)
				})

				s.And(`further events made`, func(s *testcase.Spec) {
					const furtherEventsKey = `further events`
					s.Before(func(t *testcase.T) {
						entities := spec.createEntities()
						for _, entity := range entities {
							require.Nil(t, spec.Subject.Create(getContext(t), entity))
							id, _ := resources.LookupID(entity)
							t.Defer(spec.Subject.DeleteByID, getContext(t), spec.EntityType, id)
						}
						t.Let(furtherEventsKey, toBaseValues(entities))
					})

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						requireContainsList(t, subscriber(t).events, t.I(eventsKey), `missing old events`)
						requireContainsList(t, subscriber(t).events, t.I(furtherEventsKey), `missing new events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						requireNotContainsList(t, othSubscriber(t).events, t.I(eventsKey))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						requireContainsList(t, othSubscriber(t).events, t.I(furtherEventsKey))
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
						require.ElementsMatch(t, t.I(eventsKey), subscriber(t).events)
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

func (spec CreatorPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec CreatorPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.EntityType)
}

func (spec CreatorPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
