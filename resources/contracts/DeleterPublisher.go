package contracts

import (
	"context"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type DeleterPublisher struct {
	Subject interface {
		minimumRequirements
		resources.DeleterPublisher
	}
	T              interface{}
	FixtureFactory FixtureFactory
}

func (spec DeleterPublisher) Test(t *testing.T) {
	spec.spec(t)
}

func (spec DeleterPublisher) Benchmark(b *testing.B) {
	spec.spec(b)
}

func (spec DeleterPublisher) spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	const name = `DeleterPublisher`
	s.Context(name, func(s *testcase.Spec) {
		s.Describe(`#SubscribeToDeleteByID`, spec.specSubscribeToDeleteByID)
		s.Describe(`#SubscribeToDeleteAll`, spec.specSubscribeToDeleteAll)
	}, testcase.Group(name))
}

func (spec DeleterPublisher) specSubscribeToDeleteByID(s *testcase.Spec) {
	subject := func(t *testcase.T) (resources.Subscription, error) {
		subscription, err := spec.Subject.SubscribeToDeleteByID(ctxGet(t), spec.T, subscriberGet(t))
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
		entityPtr := spec.createEntity()
		CreateEntity(t, spec.Subject, ctxGet(t), entityPtr)
		return entityPtr
	}).EagerLoading(s)

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
		Waiter.Wait()
		require.Empty(t, subscriberGet(t).Events())
	})

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			DeleteEntity(t, spec.Subject, ctxGet(t), entity.Get(t))

			Waiter.While(func() bool {
				return subscriberGet(t).EventsLen() < 1
			})
		})

		s.Then(`subscriberGet receive the delete event where ID can be located`, func(t *testcase.T) {
			spec.hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
		})

		s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, t.I(subscriptionKey).(resources.Subscription).Close())
			})

			s.And(`more events made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					entityPtr := spec.createEntity()
					CreateEntity(t, spec.Subject, ctxGet(t), entityPtr)
					DeleteEntity(t, spec.Subject, ctxGet(t), entityPtr)
					Waiter.Wait()
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
				sub, err := spec.Subject.SubscribeToDeleteByID(ctxGet(t), spec.T, othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriberGet still received the old delete event`, func(t *testcase.T) {
				require.Len(t, subscriberGet(t).Events(), 1)
				expectedID, _ := resources.LookupID(entity.Get(t))
				actualID, _ := resources.LookupID(subscriberGet(t).Events()[0])
				require.Equal(t, expectedID, actualID)
			})

			s.Then(`new subscriberGet do not receive any events`, func(t *testcase.T) {
				require.Empty(t, othSubscriber(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				const furtherEventKey = `further event`
				furtherEvent := s.Let(furtherEventKey, func(t *testcase.T) interface{} {
					t.Log(`given an another entity is stored`)
					entityPtr := spec.createEntity()
					CreateEntity(t, spec.Subject, ctxGet(t), entityPtr)
					DeleteEntity(t, spec.Subject, ctxGet(t), entityPtr)
					Waiter.While(func() bool {
						return subscriberGet(t).EventsLen() < 2
					})
					Waiter.While(func() bool {
						return getSubscriber(t, othSubscriberKey).EventsLen() < 1
					})
					return toBaseValue(entityPtr)
				}).EagerLoading(s)

				s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
					spec.hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
					spec.hasDeleteEntity(t, subscriberGet(t).Events, furtherEvent.Get(t))
				})

				s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
					spec.doesNotHaveDeleteEntity(t, othSubscriber(t).Events, entity.Get(t))
				})

				s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
					spec.hasDeleteEntity(t, subscriberGet(t).Events, furtherEvent.Get(t))
				})
			})
		})
	})
}

func (spec DeleterPublisher) specSubscribeToDeleteAll(s *testcase.Spec) {
	subject := func(t *testcase.T) (resources.Subscription, error) {
		subscription, err := spec.Subject.SubscribeToDeleteAll(ctxGet(t), spec.T, subscriberGet(t))
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

	s.Let(subscriberKey, func(t *testcase.T) interface{} {
		return newEventSubscriber(t)
	})

	ctx.Let(s, func(t *testcase.T) interface{} {
		return spec.context()
	})

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
		require.Empty(t, subscriberGet(t).Events())
	})

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.DeleteAll(ctxGet(t), spec.T))
			Waiter.While(func() bool {
				return subscriberGet(t).EventsLen() < 1
			})
		})

		s.Then(`subscriberGet receive the delete event where ID can be located`, func(t *testcase.T) {
			require.Contains(t, subscriberGet(t).Events(), spec.T)
		})

		s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
			const othSubscriberKey = `oth-subscriberGet`
			othSubscriber := func(t *testcase.T) *eventSubscriber {
				return getSubscriber(t, othSubscriberKey)
			}
			s.Before(func(t *testcase.T) {
				othSubscriber := newEventSubscriber(t)
				t.Let(othSubscriberKey, othSubscriber)
				sub, err := spec.Subject.SubscribeToDeleteAll(ctxGet(t), spec.T, othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriberGet still received the old delete event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), spec.T)
			})

			s.Then(`new subscriberGet do not receive any events`, func(t *testcase.T) {
				Waiter.Wait()
				require.Empty(t, othSubscriber(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.DeleteAll(ctxGet(t), spec.T))
					Waiter.While(func() bool {
						return subscriberGet(t).EventsLen() < 2
					})
					Waiter.While(func() bool {
						return getSubscriber(t, othSubscriberKey).EventsLen() < 1
					})
				})

				s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), spec.T)
					require.Len(t, subscriberGet(t).Events(), 2)
				})

				s.Then(`new subscriberGet only receive events made after the subscription`, func(t *testcase.T) {
					require.Contains(t, othSubscriber(t).Events(), spec.T)
					require.Len(t, othSubscriber(t).Events(), 1)
				})
			})
		})
	})
}

func (spec DeleterPublisher) hasDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, entity := range getList() {
			expectedID, _ := resources.LookupID(entity)
			actualID, _ := resources.LookupID(e)
			if expectedID == actualID {
				matchingIDFound = true
				break
			}
		}
		require.True(tb, matchingIDFound, `it was expected to includes the delete event entry`)
	})
}

func (spec DeleterPublisher) doesNotHaveDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, entity := range getList() {
			expectedID, _ := resources.LookupID(entity)
			actualID, _ := resources.LookupID(e)
			if expectedID == actualID {
				matchingIDFound = true
				break
			}
		}
		require.False(tb, matchingIDFound, `it was expected to doesn't have the delete event entry`)
	})
}

func (spec DeleterPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec DeleterPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.T)
}

func (spec DeleterPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
