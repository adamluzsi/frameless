package contracts

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"
)

type UpdaterPublisher struct {
	T
	Subject func(testing.TB) UpdaterPublisherSubject
	FixtureFactory
}

type UpdaterPublisherSubject interface {
	CRD
	frameless.Updater
	frameless.UpdaterPublisher
}

func (spec UpdaterPublisher) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return spec.Subject(t)
		},
	}
}

func (spec UpdaterPublisher) resourceGet(t *testcase.T) UpdaterPublisherSubject {
	return spec.resource().Get(t).(UpdaterPublisherSubject)
}

func (spec UpdaterPublisher) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec UpdaterPublisher) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec UpdaterPublisher) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	spec.resource().Let(s, nil)
	const name = `UpdaterPublisher`
	s.Context(name, func(s *testcase.Spec) {
		s.Describe(`#SubscribeToUpdate`, func(s *testcase.Spec) {
			subject := func(t *testcase.T) (frameless.Subscription, error) {
				subscription, err := spec.resourceGet(t).SubscribeToUpdate(ctxGet(t), spec.T, subscriberGet(t))
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
				return spec.Context()
			})

			s.Let(subscriberKey, func(t *testcase.T) interface{} {
				return newEventSubscriber(t)
			})

			const entityKey = `entity`
			entity := s.Let(entityKey, func(t *testcase.T) interface{} {
				ptr := spec.createEntity()
				CreateEntity(t, spec.resourceGet(t), ctxGet(t), ptr)
				return ptr
			}).EagerLoading(s)
			getID := func(t *testcase.T) interface{} {
				id, _ := extid.Lookup(entity.Get(t))
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
					require.Nil(t, extid.Set(entityWithNewValuesPtr, getID(t)))
					UpdateEntity(t, spec.resourceGet(t), ctxGet(t), entityWithNewValuesPtr)
					Waiter.While(func() bool { return subscriberGet(t).EventsLen() < 1 })
					return toBaseValue(entityWithNewValuesPtr)
				}).EagerLoading(s)

				s.Then(`subscriberGet receive the event`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), updatedEntity.Get(t))
				})

				s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						require.Nil(t, t.I(subscriptionKey).(frameless.Subscription).Close())
					})

					s.And(`more events made`, func(s *testcase.Spec) {
						s.Before(func(t *testcase.T) {
							id, _ := extid.Lookup(t.I(entityKey))
							updatedEntityPtr := spec.createEntity()
							require.Nil(t, extid.Set(updatedEntityPtr, id))
							require.Nil(t, spec.resourceGet(t).Update(ctxGet(t), updatedEntityPtr))
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
						sub, err := spec.resourceGet(t).SubscribeToUpdate(ctxGet(t), spec.T, othSubscriber)
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
							require.Nil(t, extid.Set(updatedEntityPtr, getID(t)))
							UpdateEntity(t, spec.resourceGet(t), ctxGet(t), updatedEntityPtr)
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
		})
	}, testcase.Group(name))
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
