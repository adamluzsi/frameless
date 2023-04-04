package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/internal/doubles"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/meta"
	"github.com/adamluzsi/frameless/ports/pubsub"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

var (
	_ memory.EventManager             = &memory.EventLog{}
	_ memory.EventManager             = &memory.EventLogTx{}
	_ comproto.OnePhaseCommitProtocol = &memory.EventLog{}
	_ meta.MetaAccessor               = &memory.EventLog{}
)

func TestMemory(t *testing.T) {
	SpecMemory{}.Spec(testcase.NewSpec(t))
}

type SpecMemory struct{}

func (spec SpecMemory) Spec(s *testcase.Spec) {
	spec.ctx().Bind(s)
	spec.memory().Bind(s)
	s.Describe(`.Add`, spec.SpecAdd)
	s.Describe(`.AddSubscription`, spec.SpecAddSubscription)
}

func (spec SpecMemory) memory() testcase.Var[*memory.EventLog] {
	return testcase.Var[*memory.EventLog]{
		ID: `*memory.EventLog`,
		Init: func(t *testcase.T) *memory.EventLog {
			return memory.NewEventLog()
		},
	}
}

func (spec SpecMemory) memoryGet(t *testcase.T) *memory.EventLog {
	return spec.memory().Get(t)
}

func (spec SpecMemory) ctx() testcase.Var[context.Context] {
	return testcase.Var[context.Context]{
		ID: `context.Context`,
		Init: func(t *testcase.T) context.Context {
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
		event = testcase.Let(s, func(t *testcase.T) interface{} {
			return AddTestEvent{V: `hello world`}
		})
		eventGet = func(t *testcase.T) memory.Event {
			return event.Get(t).(memory.Event)
		}
		subject = func(t *testcase.T) error {
			return spec.memoryGet(t).Append(spec.ctxGet(t), eventGet(t))
		}
	)

	s.When(`context is canceled`, func(s *testcase.Spec) {
		spec.ctx().Let(s, func(t *testcase.T) context.Context {
			c, cancel := context.WithCancel(context.Background())
			cancel()
			return c
		})

		s.Then(`atomic returns with context canceled error`, func(t *testcase.T) {
			assert.Must(t).ErrorIs(context.Canceled, subject(t))
		})
	})

	s.When(`during transaction`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			tx, err := spec.memoryGet(t).BeginTx(spec.ctxGet(t))
			assert.Must(t).Nil(err)
			spec.ctx().Set(t, tx)
		})

		s.Then(`Add will execute in the scope of transaction`, func(t *testcase.T) {
			assert.Must(t).Nil(subject(t))
			assert.Must(t).NotContain(spec.memoryGet(t).Events(), eventGet(t))
			assert.Must(t).Nil(spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
			assert.Must(t).Contain(spec.memoryGet(t).Events(), eventGet(t))
		})
	})
}

func (spec SpecMemory) SpecAddSubscription(s *testcase.Spec) {
	handledEvents := testcase.Let(s, func(t *testcase.T) interface{} {
		return []memory.Event{}
	})
	subscriber := testcase.Let(s, func(t *testcase.T) memory.EventLogSubscriber {
		return doubles.StubSubscriber[any, any]{
			HandleFunc: func(ctx context.Context, event memory.Event) error {
				testcase.Append(t, handledEvents, event)
				return nil
			},
			ErrorFunc: func(ctx context.Context, err error) error {
				return nil
			},
		}
	})
	subject := func(t *testcase.T) (pubsub.Subscription, error) {
		return spec.memoryGet(t).Subscribe(spec.ctxGet(t), subscriber.Get(t))
	}
	onSuccess := func(t *testcase.T) pubsub.Subscription {
		subscription, err := subject(t)
		assert.Must(t).Nil(err)
		t.Defer(subscription.Close)
		return subscription
	}

	type (
		TestEvent struct{ V int }
	)

	s.When(`events added to the *memory.EventLog`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.Must(t).Nil(spec.memoryGet(t).Append(spec.ctxGet(t), TestEvent{V: 42}))
		})

		s.Then(`since there wasn't any subscription, nothing is received in the subscriber`, func(t *testcase.T) {
			assert.Must(t).Empty(handledEvents.Get(t))
		})
	})

	s.When(`subscription is made`, func(s *testcase.Spec) {
		subscription := testcase.Let(s, func(t *testcase.T) interface{} {
			t.Log(`given the subscription is made`)
			return onSuccess(t)
		}).EagerLoading(s)
		_ = subscription

		s.And(`event is added to *memory.EventLog`, func(s *testcase.Spec) {
			expected := TestEvent{V: 42}

			s.Before(func(t *testcase.T) {
				t.Log(`and event added to the event store`)
				m := spec.memoryGet(t)
				assert.Must(t).Nil(m.Append(spec.ctxGet(t), expected))
				waiter.Wait()
			})

			s.Then(`events will be emitted to the subscriber`, func(t *testcase.T) {
				eventually.Assert(t, func(tb assert.It) {
					assert.Must(tb).Contain(handledEvents.Get(t), expected)
				})
			})

			s.And(`during transaction`, func(s *testcase.Spec) {
				spec.ctx().Let(s, func(t *testcase.T) context.Context {
					c := spec.ctx().Init(t).(context.Context)
					tx, err := spec.memoryGet(t).BeginTx(c)
					assert.Must(t).Nil(err)
					t.Defer(spec.memoryGet(t).RollbackTx, tx)
					return tx
				})

				s.Then(`no events will emitted during the transaction`, func(t *testcase.T) {
					waiter.Wait()
					assert.Must(t).Empty(handledEvents.Get(t))
				})

				s.And(`after commit`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						assert.Must(t).Nil(spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
					})

					s.Then(`event(s) will be emitted`, func(t *testcase.T) {
						eventually.Assert(t, func(tb assert.It) {
							assert.Must(tb).Contain(handledEvents.Get(t), expected)
						})
					})
				})

				s.And(`after rollback`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						assert.Must(t).Nil(spec.memoryGet(t).RollbackTx(spec.ctxGet(t)))
					})

					s.Then(`no event(s) will emitted after the transaction`, func(t *testcase.T) {
						waiter.Wait()
						assert.Must(t).Empty(handledEvents.Get(t))
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
	eventLog := memory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = true

	sub, err := eventLog.Subscribe(ctx, doubles.StubSubscriber[any, any]{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			if ent == (BEvent{}) {
				return nil
			}
			return eventLog.Append(ctx, BEvent{})
		},
	})
	assert.Must(t).Nil(err)
	defer sub.Close()
	assert.Must(t).Nil(eventLog.Append(ctx, AEvent{}))
}
