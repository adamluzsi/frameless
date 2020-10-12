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
	T              interface{}
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
		subject := func(t *testcase.T) (resources.Subscription, error) {
			subscription, err := spec.Subject.SubscribeToCreate(getContext(t), spec.T, subscriber(t))
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
			require.Empty(t, subscriber(t).Events())
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
					t.Defer(spec.Subject.DeleteByID, spec.context(), spec.T, id)
				}
				t.Let(eventsKey, toBaseValues(entities))

				WaitWhile(func() bool {
					return subscriber(t).EventsLen() < len(entities)
				})
			})

			s.Then(`subscriber receive those events`, func(t *testcase.T) {
				require.ElementsMatch(t, t.I(eventsKey).([]interface{}), subscriber(t).Events())
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
							t.Defer(spec.Subject.DeleteByID, getContext(t), spec.T, id)
						}

						WaitWhile(func() bool {
							return subscriber(t).EventsLen() < len(t.I(eventsKey).([]interface{}))
						})
					})

					s.Then(`handler don't receive the new events`, func(t *testcase.T) {
						require.ElementsMatch(t, t.I(eventsKey).([]interface{}), subscriber(t).Events())
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
					newSubscription, err := spec.Subject.SubscribeToCreate(getContext(t), spec.T, othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, newSubscription)
					t.Defer(newSubscription.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					require.ElementsMatch(t, subscriber(t).Events(), t.I(eventsKey).([]interface{}))
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					t.Log(`new subscriber don't have the vents since it subscribed after events had been already fired`)
					Wait() // Wait a little to receive events if we receive any
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`further events made`, func(s *testcase.Spec) {
					const furtherEventsKey = `further events`
					s.Before(func(t *testcase.T) {
						entities := spec.createEntities()
						for _, entity := range entities {
							require.Nil(t, spec.Subject.Create(getContext(t), entity))
							id, _ := resources.LookupID(entity)
							t.Defer(spec.Subject.DeleteByID, getContext(t), spec.T, id)
						}
						t.Let(furtherEventsKey, toBaseValues(entities))

						WaitWhile(func() bool {
							return subscriber(t).EventsLen() < len(t.I(eventsKey).([]interface{}))+len(t.I(furtherEventsKey).([]interface{}))
						})

						WaitWhile(func() bool {
							return othSubscriber(t).EventsLen() < len(t.I(furtherEventsKey).([]interface{}))
						})
					})

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						requireContainsList(t, subscriber(t).Events(), t.I(eventsKey), `missing old events`)
						requireContainsList(t, subscriber(t).Events(), t.I(furtherEventsKey), `missing new events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						requireNotContainsList(t, othSubscriber(t).Events(), t.I(eventsKey))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						requireContainsList(t, othSubscriber(t).Events(), t.I(furtherEventsKey))
					})
				})
			})
		})

		s.Describe(`relationship with OnePhaseCommitProtocol`, spec.specOnePhaseCommitProtocol)
	})
}

func (spec CreatorPublisher) specOnePhaseCommitProtocol(s *testcase.Spec) {
	res, ok := spec.Subject.(resources.OnePhaseCommitProtocol)
	if !ok {
		return
	}

	const eventsKey = `events`

	s.Before(func(t *testcase.T) {
		entities := spec.createEntities()
		for _, entity := range entities {
			require.Nil(t, spec.Subject.Create(getContext(t), entity))
			id, _ := resources.LookupID(entity)
			// we use a new context here to enforce that the cleaning will be done outside of any context.
			// It might fail but will ensure proper cleanup.
			t.Defer(spec.Subject.DeleteByID, spec.context(), spec.T, id)
		}
		t.Let(eventsKey, toBaseValues(entities))
	})

	s.Let(contextKey, func(t *testcase.T) interface{} {
		t.Log(`given we are in transaction`)
		ctxInTx, err := res.BeginTx(spec.context())
		require.Nil(t, err)
		t.Defer(res.RollbackTx, ctxInTx)
		return ctxInTx
	})

	s.Then(`before a commit, events will be absent`, func(t *testcase.T) {
		Wait()
		require.Empty(t, subscriber(t).Events())
		require.Nil(t, res.CommitTx(getContext(t)))
	})

	s.Then(`after a commit, events will be present`, func(t *testcase.T) {
		require.Nil(t, res.CommitTx(getContext(t)))
		WaitWhile(func() bool {
			return subscriber(t).EventsLen() < len(t.I(eventsKey).([]interface{}))
		})
		require.ElementsMatch(t, t.I(eventsKey), subscriber(t).Events())
	})

	s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
		require.Nil(t, res.RollbackTx(getContext(t)))
		Wait()
		require.Empty(t, subscriber(t).Events())
	})
}

func (spec CreatorPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec CreatorPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.T)
}

func (spec CreatorPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
