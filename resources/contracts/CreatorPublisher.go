package contracts

import (
	"testing"

	"github.com/adamluzsi/testcase"
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
	spec.spec(t)
}

func (spec CreatorPublisher) Benchmark(b *testing.B) {
	spec.spec(b)
}

func (spec CreatorPublisher) spec(tb testing.TB) {
	const name = `CreatorPublisher`
	testcase.NewSpec(tb).Context(name, func(s *testcase.Spec) {
		debug(s, spec.Subject)
		s.Describe(`#SubscribeToCreate`, func(s *testcase.Spec) {
			subject := func(t *testcase.T) (resources.Subscription, error) {
				subscription, err := spec.Subject.SubscribeToCreate(ctxGet(t), spec.T, subscriberGet(t))
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

			ctxLetWithFixtureFactory(s, spec.FixtureFactory)

			s.Let(subscriberKey, func(t *testcase.T) interface{} {
				return newEventSubscriber(t)
			})

			s.Before(func(t *testcase.T) {
				t.Log(`given a subscription is made`)
				require.NotNil(t, onSuccess(t))
			})

			s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
				require.Empty(t, subscriberGet(t).Events())
			})

			s.And(`events made`, func(s *testcase.Spec) {
				events := s.Let(`events`, func(t *testcase.T) interface{} {
					entities := genEntities(spec.FixtureFactory, spec.T)

					for _, entity := range entities {
						CreateEntity(t, spec.Subject, ctxGet(t), entity)
					}

					// wait until the subscriberGet received the events
					Waiter.While(func() bool {
						return subscriberGet(t).EventsLen() < len(entities)
					})

					return toBaseValues(entities)
				}).EagerLoading(s)
				getEvents := func(t *testcase.T) []interface{} { return events.Get(t).([]interface{}) }

				s.Then(`subscriberGet receive those events`, func(t *testcase.T) {
					require.ElementsMatch(t, getEvents(t), subscriberGet(t).Events())
				})

				s.And(`subscription is cancelled by close`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						sub := t.I(subscriptionKey).(resources.Subscription)
						require.Nil(t, sub.Close())
					})

					s.And(`more events made`, func(s *testcase.Spec) {
						s.Before(func(t *testcase.T) {
							entities := genEntities(spec.FixtureFactory, spec.T)
							for _, entity := range entities {
								CreateEntity(t, spec.Subject, ctxGet(t), entity)
							}
							Waiter.Wait()
						})

						s.Then(`handler don't receive the new events`, func(t *testcase.T) {
							require.ElementsMatch(t, getEvents(t), subscriberGet(t).Events())
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
						newSubscription, err := spec.Subject.SubscribeToCreate(ctxGet(t), spec.T, othSubscriber)
						require.Nil(t, err)
						require.NotNil(t, newSubscription)
						t.Defer(newSubscription.Close)
					})

					s.Then(`original subscriberGet still receive old events`, func(t *testcase.T) {
						require.ElementsMatch(t, subscriberGet(t).Events(), getEvents(t))
					})

					s.Then(`new subscriberGet do not receive old events`, func(t *testcase.T) {
						t.Log(`new subscriberGet don't have the vents since it subscribed after events had been already fired`)
						Waiter.Wait() // Wait a little to receive events if we receive any
						require.Empty(t, othSubscriber(t).Events())
					})

					s.And(`further events made`, func(s *testcase.Spec) {
						furtherEvents := s.Let(`further events`, func(t *testcase.T) interface{} {
							entities := genEntities(spec.FixtureFactory, spec.T)
							for _, entity := range entities {
								CreateEntity(t, spec.Subject, ctxGet(t), entity)
							}

							Waiter.While(func() bool {
								return subscriberGet(t).EventsLen() < len(getEvents(t))+len(entities)
							})

							Waiter.While(func() bool {
								return othSubscriber(t).EventsLen() < len(entities)
							})

							return toBaseValues(entities)
						}).EagerLoading(s)
						getFurtherEvents := func(t *testcase.T) []interface{} { return furtherEvents.Get(t).([]interface{}) }

						s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
							requireContainsList(t, subscriberGet(t).Events(), events.Get(t), `missing old events`)
							requireContainsList(t, subscriberGet(t).Events(), getFurtherEvents(t), `missing new events`)
						})

						s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
							requireNotContainsList(t, othSubscriber(t).Events(), getEvents(t))
						})

						s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
							requireContainsList(t, othSubscriber(t).Events(), getFurtherEvents(t))
						})
					})
				})
			})
		})
	}, testcase.Group(name))
}
