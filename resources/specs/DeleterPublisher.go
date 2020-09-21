package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type DeleterPublisher struct {
	Subject interface {
		minimumRequirements
		resources.DeleterPublisher
	}
	EntityType     interface{}
	FixtureFactory FixtureFactory
}

func (spec DeleterPublisher) Test(t *testing.T) {
	t.Run(`DeleterPublisher`, func(t *testing.T) {
		spec.Spec(testcase.NewSpec(t))
	})
}

func (spec DeleterPublisher) Benchmark(b *testing.B) {
	b.Run(`DeleterPublisher`, func(b *testing.B) {
		spec.Spec(testcase.NewSpec(b))
	})
}

func (spec DeleterPublisher) Spec(s *testcase.Spec) {
	s.Describe(`#SubscribeToDeleteByID`, spec.specSubscribeToDeleteByID)
}

func (spec DeleterPublisher) specSubscribeToDeleteByID(s *testcase.Spec) {
	const contextKey = `getContext`
	const subscriberKey = `subscriber`
	const subscriptionKey = `subscription`
	getSubscriber := func(t *testcase.T, key string) *eventSubscriber {
		return t.I(key).(*eventSubscriber)
	}
	getContext := func(t *testcase.T) context.Context {
		return t.I(contextKey).(context.Context)
	}
	subscriber := func(t *testcase.T) *eventSubscriber {
		return getSubscriber(t, subscriberKey)
	}
	subject := func(t *testcase.T) (resources.Subscription, error) {
		subscription, err := spec.Subject.SubscribeToDeleteByID(getContext(t), spec.EntityType, subscriber(t))
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
		t.Log(`given an entity is stored`)
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

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			id, _ := resources.LookupID(t.I(entityKey))
			require.Nil(t, spec.Subject.DeleteByID(getContext(t), spec.EntityType, id))
		})

		s.Then(`subscriber receive the delete event where ID can be located`, func(t *testcase.T) {
			spec.hasDeleteEntity(t, subscriber(t).events, t.I(entityKey))
		})

		s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, t.I(subscriptionKey).(resources.Subscription).Close())
			})

			s.And(`more events made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					entityPtr := spec.createEntity()
					require.Nil(t, spec.Subject.Create(getContext(t), entityPtr))
					id, _ := resources.LookupID(entityPtr)
					require.Nil(t, spec.Subject.DeleteByID(getContext(t), spec.EntityType, id))
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
				sub, err := spec.Subject.SubscribeToDeleteByID(getContext(t), spec.EntityType, othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriber still received the old delete event`, func(t *testcase.T) {
				require.Len(t, subscriber(t).events, 1)
				expectedID, _ := resources.LookupID(t.I(entityKey))
				actualID, _ := resources.LookupID(subscriber(t).events[0])
				require.Equal(t, expectedID, actualID)
			})

			s.Then(`new subscriber do not receive any events`, func(t *testcase.T) {
				require.Empty(t, othSubscriber(t).events)
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				const furtherEventKey = `further event`
				s.Before(func(t *testcase.T) {
					t.Log(`given an another entity is stored`)
					entityPtr := spec.createEntity()
					require.Nil(t, spec.Subject.Create(getContext(t), entityPtr))
					id, _ := resources.LookupID(entityPtr)
					t.Let(furtherEventKey, toBaseValue(entityPtr))
					require.Nil(t, spec.Subject.DeleteByID(getContext(t), spec.EntityType, id))
				})

				s.Then(`original subscriber receives all events`, func(t *testcase.T) {
					spec.hasDeleteEntity(t, subscriber(t).events, t.I(entityKey))
					spec.hasDeleteEntity(t, subscriber(t).events, t.I(furtherEventKey))
				})

				s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
					spec.doesNotHaveDeleteEntity(t, othSubscriber(t).events, t.I(entityKey))
				})

				s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
					spec.hasDeleteEntity(t, subscriber(t).events, t.I(furtherEventKey))
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
					spec.hasDeleteEntity(t, subscriber(t).events, t.I(entityKey))
				})

				s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
					require.Nil(t, res.RollbackTx(getContext(t)))
					require.Empty(t, subscriber(t).events)
				})
			})
		}
	})
}

func (spec DeleterPublisher) specSubscribeToDeleteAll(s *testcase.Spec) {
	const contextKey = `getContext`
	const subscriberKey = `subscriber`
	const subscriptionKey = `subscription`

	getSubscriber := func(t *testcase.T, key string) *eventSubscriber {
		return t.I(key).(*eventSubscriber)
	}
	getContext := func(t *testcase.T) context.Context {
		return t.I(contextKey).(context.Context)
	}
	subscriber := func(t *testcase.T) *eventSubscriber {
		return getSubscriber(t, subscriberKey)
	}
	subject := func(t *testcase.T) (resources.Subscription, error) {
		subscription, err := spec.Subject.SubscribeToDeleteAll(getContext(t), spec.EntityType, subscriber(t))
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

	s.Let(contextKey, func(t *testcase.T) interface{} {
		return spec.context()
	})

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
		require.Empty(t, subscriber(t).events)
	})

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.DeleteAll(getContext(t), spec.EntityType))
		})

		s.Then(`subscriber receive the delete event where ID can be located`, func(t *testcase.T) {
			require.Contains(t, subscriber(t).events, spec.EntityType)
		})

		s.And(`then new subscriber registered`, func(s *testcase.Spec) {
			const othSubscriberKey = `oth-subscriber`
			othSubscriber := func(t *testcase.T) *eventSubscriber {
				return getSubscriber(t, othSubscriberKey)
			}
			s.Before(func(t *testcase.T) {
				othSubscriber := newEventSubscriber(t)
				t.Let(othSubscriberKey, othSubscriber)
				sub, err := spec.Subject.SubscribeToDeleteByID(getContext(t), spec.EntityType, othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriber still received the old delete event`, func(t *testcase.T) {
				require.Contains(t, subscriber(t).events, spec.EntityType)
			})

			s.Then(`new subscriber do not receive any events`, func(t *testcase.T) {
				require.Empty(t, othSubscriber(t).events)
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				const furtherEventKey = `further event`
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.DeleteAll(getContext(t), spec.EntityType))
				})

				s.Then(`original subscriber receives all events`, func(t *testcase.T) {
					require.Contains(t, subscriber(t).events, spec.EntityType)
					require.Len(t, subscriber(t).events, 2)
				})

				s.Then(`new subscriber only receive events made after the subscription`, func(t *testcase.T) {
					require.Contains(t, othSubscriber(t).events, spec.EntityType)
					require.Len(t, othSubscriber(t), 1)
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
					require.Contains(t, subscriber(t).events, spec.EntityType)
				})

				s.Then(`after a rollback, events will be absent`, func(t *testcase.T) {
					require.Nil(t, res.RollbackTx(getContext(t)))
					require.Empty(t, subscriber(t).events)
				})
			})
		}
	})
}

func (spec DeleterPublisher) hasDeleteEntity(tb testing.TB, list []interface{}, e interface{}) {
	var matchingIDFound bool
	for _, entity := range list {
		expectedID, _ := resources.LookupID(entity)
		actualID, _ := resources.LookupID(e)
		if expectedID == actualID {
			matchingIDFound = true
			break
		}
	}
	require.True(tb, matchingIDFound, `it was expected to includes the delete event entry`)
}

func (spec DeleterPublisher) doesNotHaveDeleteEntity(tb testing.TB, list []interface{}, e interface{}) {
	var matchingIDFound bool
	for _, entity := range list {
		expectedID, _ := resources.LookupID(entity)
		actualID, _ := resources.LookupID(e)
		if expectedID == actualID {
			matchingIDFound = true
			break
		}
	}
	require.False(tb, matchingIDFound, `it was expected to doesn't have the delete event entry`)
}

func (spec DeleterPublisher) context() context.Context {
	return spec.FixtureFactory.Context()
}

func (spec DeleterPublisher) createEntity() interface{} {
	return spec.FixtureFactory.Create(spec.EntityType)
}

func (spec DeleterPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createEntity())
	}
	return es
}
