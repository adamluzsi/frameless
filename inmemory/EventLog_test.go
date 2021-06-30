package inmemory_test

import (
	"context"
	"github.com/adamluzsi/frameless/stubs"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var (
	_ inmemory.EventManager            = &inmemory.EventLog{}
	_ inmemory.EventManager            = &inmemory.EventLogTx{}
	_ frameless.OnePhaseCommitProtocol = &inmemory.EventLog{}
	_ frameless.MetaAccessor           = &inmemory.EventLog{}
)

func TestMemory(t *testing.T) {
	SpecMemory{}.Spec(t)
}

type SpecMemory struct{}

func (spec SpecMemory) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	spec.ctx().Let(s, nil)
	spec.memory().Let(s, nil)
	s.Describe(`.Add`, spec.SpecAdd)
	s.Describe(`.AddSubscription`, spec.SpecAddSubscription)
}

func (spec SpecMemory) memory() testcase.Var {
	return testcase.Var{
		Name: `*inmemory.EventLog`,
		Init: func(t *testcase.T) interface{} {
			return inmemory.NewEventLog()
		},
	}
}

func (spec SpecMemory) memoryGet(t *testcase.T) *inmemory.EventLog {
	return spec.memory().Get(t).(*inmemory.EventLog)
}

func (spec SpecMemory) ctx() testcase.Var {
	return testcase.Var{
		Name: `context.Context`,
		Init: func(t *testcase.T) interface{} {
			return context.Background()
		},
	}
}

func (spec SpecMemory) ctxGet(t *testcase.T) context.Context {
	return spec.ctx().Get(t).(context.Context)
}

func (spec SpecMemory) SpecAdd(s *testcase.Spec) {
	type AddTestEvent struct{ V string }
	var (
		event = s.Let(`event`, func(t *testcase.T) interface{} {
			return AddTestEvent{V: `hello world`}
		})
		eventGet = func(t *testcase.T) inmemory.Event {
			return event.Get(t).(inmemory.Event)
		}
		subject = func(t *testcase.T) error {
			return spec.memoryGet(t).Append(spec.ctxGet(t), eventGet(t))
		}
	)

	s.When(`context is canceled`, func(s *testcase.Spec) {
		spec.ctx().Let(s, func(t *testcase.T) interface{} {
			c, cancel := context.WithCancel(context.Background())
			cancel()
			return c
		})

		s.Then(`atomic returns with context canceled error`, func(t *testcase.T) {
			require.Equal(t, context.Canceled, subject(t))
		})
	})

	s.When(`during transaction`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			tx, err := spec.memoryGet(t).BeginTx(spec.ctxGet(t))
			require.Nil(t, err)
			spec.ctx().Set(t, tx)
		})

		s.Then(`Add will execute in the scope of transaction`, func(t *testcase.T) {
			require.NoError(t, subject(t))
			require.NotContains(t, spec.memoryGet(t).Events(), eventGet(t))
			require.NoError(t, spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
			require.Contains(t, spec.memoryGet(t).Events(), eventGet(t))
		})
	})
}

func (spec SpecMemory) SpecAddSubscription(s *testcase.Spec) {
	handledEvents := s.Let(`handled inmemory.Event`, func(t *testcase.T) interface{} {
		return []inmemory.Event{}
	})
	subscriber := s.Let(`inmemory.MemorySubscriber`, func(t *testcase.T) interface{} {
		return stubs.Subscriber{
			HandleFunc: func(ctx context.Context, event inmemory.Event) error {
				testcase.Append(t, handledEvents, event)
				return nil
			},
			ErrorFunc: func(ctx context.Context, err error) error {
				return nil
			},
		}
	})
	subscriberGet := func(t *testcase.T) frameless.Subscriber {
		return subscriber.Get(t).(frameless.Subscriber)
	}
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		return spec.memoryGet(t).Subscribe(spec.ctxGet(t), subscriberGet(t))
	}
	onSuccess := func(t *testcase.T) frameless.Subscription {
		subscription, err := subject(t)
		require.Nil(t, err)
		t.Defer(subscription.Close)
		return subscription
	}

	type (
		TestEvent struct{ V int }
	)

	s.When(`events added to the *inmemory.EventLog`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.memoryGet(t).Append(spec.ctxGet(t), TestEvent{V: 42}))
		})

		s.Then(`since there wasn't any subscription, nothing is received in the subscriber`, func(t *testcase.T) {
			require.Empty(t, handledEvents.Get(t))
		})
	})

	s.When(`subscription is made`, func(s *testcase.Spec) {
		subscription := s.Let(`Subscription`, func(t *testcase.T) interface{} {
			t.Log(`given the subscription is made`)
			return onSuccess(t)
		}).EagerLoading(s)
		_ = subscription

		s.And(`event is added to *inmemory.EventLog`, func(s *testcase.Spec) {
			expected := TestEvent{V: 42}

			s.Before(func(t *testcase.T) {
				t.Log(`and event added to the event store`)
				m := spec.memoryGet(t)
				require.Nil(t, m.Append(spec.ctxGet(t), expected))
				waiter.Wait()
			})

			s.Then(`events will be emitted to the subscriber`, func(t *testcase.T) {
				retry.Assert(t, func(tb testing.TB) {
					require.Contains(tb, handledEvents.Get(t), expected)
				})
			})

			s.And(`during transaction`, func(s *testcase.Spec) {
				spec.ctx().Let(s, func(t *testcase.T) interface{} {
					c := spec.ctx().Init(t).(context.Context)
					tx, err := spec.memoryGet(t).BeginTx(c)
					require.Nil(t, err)
					t.Defer(spec.memoryGet(t).RollbackTx, tx)
					return tx
				})

				s.Then(`no events will emitted during the transaction`, func(t *testcase.T) {
					waiter.Wait()
					require.Empty(t, handledEvents.Get(t))
				})

				s.And(`after commit`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						require.Nil(t, spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
					})

					s.Then(`event(s) will be emitted`, func(t *testcase.T) {
						retry.Assert(t, func(tb testing.TB) {
							require.Contains(tb, handledEvents.Get(t), expected)
						})
					})
				})

				s.And(`after rollback`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						require.Nil(t, spec.memoryGet(t).RollbackTx(spec.ctxGet(t)))
					})

					s.Then(`no event(s) will emitted after the transaction`, func(t *testcase.T) {
						waiter.Wait()
						require.Empty(t, handledEvents.Get(t))
					})
				})
			})
		})
	})
}

func TestEventLog_optionsDisabledAsyncSubscriptionHandling_subscriptionCanAppendEvents(t *testing.T) {
	type (
		AEvent struct{}
		BEvent struct{}
	)

	ctx := context.Background()
	eventLog := inmemory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = true

	sub, err := eventLog.Subscribe(ctx, stubs.Subscriber{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			if ent == (BEvent{}) {
				return nil
			}
			return eventLog.Append(ctx, BEvent{})
		},
	})
	require.Nil(t, err)
	defer sub.Close()
	require.Nil(t, eventLog.Append(ctx, AEvent{}))
}
