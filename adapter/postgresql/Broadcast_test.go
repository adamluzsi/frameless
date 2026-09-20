package postgresql_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestBroadcast(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ctx        = let.Var(s, func(t *testcase.T) context.Context { return broadcastContext(t) })
		poolSize   = let.Var(s, func(t *testcase.T) int32 { return 16 })
		connection = let.Var(s, func(t *testcase.T) postgresql.Connection { return queueV2Connection(t, poolSize.Get(t)) })
		name       = let.Var(s, func(t *testcase.T) string { return "broadcast_" + strings.ReplaceAll(t.Random.UUID(), "-", "") })
		stateless  = let.Var(s, func(t *testcase.T) bool { return false })
		encoding   = let.Var[codec.Codec](s, func(t *testcase.T) codec.Codec { return nil })
		data       = let.Var(s, func(t *testcase.T) broadcastPayload { return makeBroadcastPayload(t) })
		subject    = let.Var(s, func(t *testcase.T) *postgresql.Broadcast[broadcastPayload] {
			return &postgresql.Broadcast[broadcastPayload]{
				Connection:         connection.Get(t),
				Name:               name.Get(t),
				Codec:              encoding.Get(t),
				StatelessSubscribe: stateless.Get(t),
			}
		})
	)

	s.Describe("#Migrate", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Migrate(ctx.Get(t)) }

		s.Test("repeatedly provisions the legacy feed tables", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			var exists bool
			err := connection.Get(t).DB.QueryRow(ctx.Get(t), `SELECT
				to_regclass('frameless_workflow_notification_channels') IS NOT NULL AND
				to_regclass('frameless_workflow_notification_subscribers') IS NOT NULL AND
				to_regclass('frameless_workflow_notification_events') IS NOT NULL`).Scan(&exists)
			assert.NoError(t, err)
			assert.True(t, exists)
		})
	})

	modeSpec := func(s *testcase.Spec) {
		s.Describe("pubsub contracts", func(s *testcase.Spec) {
			act := func(t *testcase.T) {
				b := subject.Get(t)
				cfg := pubsubcontract.Config[broadcastPayload]{
					MakeContext:                       broadcastContext,
					MakeData:                          makeBroadcastPayload,
					SupportPublishContextCancellation: true,
				}
				testcase.RunSuite(testcase.NewSpec(t.TB),
					pubsubcontract.Volatile[broadcastPayload](b, b, cfg),
					pubsubcontract.Broadcast[broadcastPayload](b, func(tb testing.TB) pubsub.Subscriber[broadcastPayload] {
						return bindBroadcastSubscriber(tb, context.Background(), b)
					}, cfg),
				)
			}

			s.Test("provides volatile delivery and fan-out of concrete payloads", act)
		})

		s.Describe("#Publish", func(s *testcase.Spec) {
			act := func(t *testcase.T) error { return subject.Get(t).Publish(ctx.Get(t), data.Get(t)) }
			marshalError := let.Error(s)

			roundTrip := func(t *testcase.T) {
				b := subject.Get(t)
				sub := pullBroadcastSubscription(t, ctx.Get(t), b)
				// Keep a polling cursor idle so the wire payload remains observable,
				// including when the consumer under test uses LISTEN.
				bindBroadcastSubscriber(t, ctx.Get(t), &postgresql.Broadcast[broadcastPayload]{
					Connection: b.Connection, Name: b.Name, StatelessSubscribe: true,
				})
				assert.Must(t).NoError(act(t))

				var wire []byte
				assert.Must(t).NoError(b.Connection.DB.QueryRow(ctx.Get(t),
					`SELECT data FROM frameless_workflow_notification_events`).Scan(&wire))
				c := encoding.Get(t)
				if c == nil {
					c = jsonkit.Codec{}
				}
				want, err := c.Marshal(data.Get(t))
				assert.Must(t).NoError(err)
				assert.Equal(t, wire, want)
				assert.Equal(t, sub.Receive(t).Data(), data.Get(t))
			}

			s.Test("defaults to JSON and round-trips a concrete non-workflow payload", roundTrip)

			s.When("a custom codec is configured", func(s *testcase.Spec) {
				encoding.LetValue(s, broadcastEnvelopeCodec{})

				s.Then("uses its wire format for both encoding and decoding", roundTrip)
			})

			s.When("the codec cannot marshal the payload", func(s *testcase.Spec) {
				encoding.Let(s, func(t *testcase.T) codec.Codec {
					err := marshalError.Get(t)
					return struct {
						codec.Marshaler
						codec.Unmarshaler
					}{codec.MarshalerFunc(func(any) ([]byte, error) { return nil, err }), jsonkit.Codec{}}
				})

				s.Then("preserves the marshal error for errors.Is", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), marshalError.Get(t))
				})
			})

			s.When("Name is omitted", func(s *testcase.Spec) {
				name.LetValue(s, "")

				s.Then("publishes on frameless_broadcast", func(t *testcase.T) {
					peer := &postgresql.Broadcast[broadcastPayload]{
						Connection: connection.Get(t), Name: "frameless_broadcast",
						StatelessSubscribe: stateless.Get(t),
					}
					sub := pullBroadcastSubscription(t, ctx.Get(t), peer)
					assert.Must(t).NoError(act(t))
					assert.Equal(t, sub.Receive(t).Data(), data.Get(t))
				})
			})
		})

		s.Describe("#Subscribe", func(s *testcase.Spec) {
			subscription := let.Var(s, func(t *testcase.T) *broadcastSubscription[broadcastPayload] {
				return pullBroadcastSubscription(t, ctx.Get(t), subject.Get(t))
			})
			act := func(t *testcase.T) (pubsub.Message[broadcastPayload], error, bool) {
				return subscription.Get(t).Next()
			}
			decodeError := let.Error(s)

			s.Before(func(t *testcase.T) {
				subscription.Get(t)
				assert.Must(t).NoError(subject.Get(t).Publish(ctx.Get(t), data.Get(t)))
			})

			s.Test("delivers a concrete message with harmless ACK and NACK", func(t *testcase.T) {
				msg, err, ok := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).True(ok)
				assert.Must(t).NotNil(msg)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.NACK())
				assert.NoError(t, msg.ACK())
				assert.NoError(t, msg.NACK())
			})

			s.When("the codec cannot unmarshal the payload", func(s *testcase.Spec) {
				encoding.Let(s, func(t *testcase.T) codec.Codec {
					err := decodeError.Get(t)
					return struct {
						codec.Marshaler
						codec.Unmarshaler
					}{jsonkit.Codec{}, codec.UnmarshalerFunc(func([]byte, any) error { return err })}
				})

				s.Then("yields the decode error instead of a partial message", func(t *testcase.T) {
					msg, err, ok := act(t)
					assert.True(t, ok)
					assert.Nil(t, msg)
					assert.ErrorIs(t, err, decodeError.Get(t))
				})
			})
		})
	}

	modeSpec(s)
	s.When("StatelessSubscribe is enabled", func(s *testcase.Spec) {
		stateless.LetValue(s, true)
		modeSpec(s)
	})

	s.Describe("stateless delayed fan-out", func(s *testcase.Spec) {
		poolSize.LetValue(s, 1)
		stateless.LetValue(s, true)
		batch := let.Var(s, func(t *testcase.T) []broadcastPayload {
			return []broadcastPayload{makeBroadcastPayload(t), makeBroadcastPayload(t), makeBroadcastPayload(t)}
		})
		act := func(t *testcase.T) *broadcastSubscription[broadcastPayload] {
			return pullBroadcastSubscription(t, ctx.Get(t), subject.Get(t))
		}

		s.Test("binds several idle subscribers without reserving the only connection or losing their messages", func(t *testcase.T) {
			subs := []*broadcastSubscription[broadcastPayload]{act(t), act(t), act(t)}
			for _, value := range batch.Get(t) {
				assert.Must(t).NoError(subject.Get(t).Publish(ctx.Get(t), value))
			}
			// None of the iterators has started; all publications precede consumption.
			for _, sub := range subs {
				for _, value := range batch.Get(t) {
					assert.Equal(t, sub.Receive(t).Data(), value)
				}
			}
		})
	})

	s.Describe("WorkflowNotificationBroadcast interoperability", func(s *testcase.Spec) {
		var (
			wrapperName      = let.Var(s, func(t *testcase.T) string { return name.Get(t) })
			genericName      = let.Var(s, func(t *testcase.T) string { return name.Get(t) })
			wrapperStateless = let.Var(s, func(t *testcase.T) bool { return false })
			wrapper          = let.Var(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				return &postgresql.WorkflowNotificationBroadcast{
					Connection:              queueV2ConnectPool(t, connection.Get(t).DB.Config()),
					Name:                    wrapperName.Get(t),
					StatelessSubscribe:      wrapperStateless.Get(t),
					PollInterval:            10 * time.Millisecond,
					SubscriberLeaseDuration: 2 * time.Second,
				}
			})
			generic = let.Var(s, func(t *testcase.T) *postgresql.Broadcast[workflow.Notification] {
				return &postgresql.Broadcast[workflow.Notification]{
					Connection:              connection.Get(t),
					Name:                    genericName.Get(t),
					Codec:                   wfjson.NewCodec(),
					StatelessSubscribe:      !wrapperStateless.Get(t),
					PollInterval:            10 * time.Millisecond,
					SubscriberLeaseDuration: 2 * time.Second,
				}
			})
			notifications = let.Var(s, func(t *testcase.T) []workflow.Notification {
				return []workflow.Notification{
					workflow.ProcessSchedule{ProcessID: wftest.MakeProcessID(t)},
					workflow.ProcessCancel{ProcessID: wftest.MakeProcessID(t)},
				}
			})
		)
		act := func(t *testcase.T) error {
			return errors.Join(
				wrapper.Get(t).Publish(ctx.Get(t), notifications.Get(t)[0]),
				generic.Get(t).Publish(ctx.Get(t), notifications.Get(t)[1]),
			)
		}

		exchange := func(t *testcase.T) {
			w, b := wrapper.Get(t), generic.Get(t)
			assert.Must(t).NoError(w.Migrate(ctx.Get(t)))
			fromWrapper := pullBroadcastSubscription(t, ctx.Get(t), w)
			fromGeneric := pullBroadcastSubscription(t, ctx.Get(t), b)
			assert.Eventually(t, time.Second, func(it testing.TB) {
				var wrapperConnections, genericConnections int32 = 1, 0
				if wrapperStateless.Get(t) {
					wrapperConnections, genericConnections = 0, 1
				}
				assert.Equal(it, w.Connection.DB.Stat().AcquiredConns(), wrapperConnections)
				assert.Equal(it, b.Connection.DB.Stat().AcquiredConns(), genericConnections)
			})
			assert.Must(t).NoError(act(t))
			for _, notification := range notifications.Get(t) {
				assert.Equal(t, fromWrapper.Receive(t).Data(), notification)
				assert.Equal(t, fromGeneric.Receive(t).Data(), notification)
			}
		}

		s.Test("exchanges both notification types in both directions on an explicit name", exchange)

		s.When("the wrapper polls and the generic broadcast uses LISTEN", func(s *testcase.Spec) {
			wrapperStateless.LetValue(s, true)
			s.Then("exchanges notifications in both directions", exchange)
		})

		s.When("the wrapper uses its default workflow name", func(s *testcase.Spec) {
			wrapperName.LetValue(s, "")
			genericName.LetValue(s, "frameless_workflow_notifications")
			s.Then("exchanges notifications on frameless_workflow_notifications rather than the generic default", exchange)

			s.And("the wrapper polls and the generic broadcast uses LISTEN", func(s *testcase.Spec) {
				wrapperStateless.LetValue(s, true)
				s.Then("still exchanges notifications in both directions", exchange)
			})
		})
	})
}

type broadcastPayload struct {
	ID     string            `json:"id"`
	Count  int               `json:"count"`
	Labels map[string]string `json:"labels"`
}

func makeBroadcastPayload(tb testing.TB) broadcastPayload {
	tb.Helper()
	t := testcase.ToT(&tb)
	return broadcastPayload{
		ID: t.Random.UUID(), Count: t.Random.IntBetween(1, 1000),
		Labels: map[string]string{"text": "árvíztűrő\n\"" + t.Random.UUID()},
	}
}

// A distinct, PostgreSQL NOTIFY-safe wire format, not just a JSON codec alias.
type broadcastEnvelopeCodec struct{}

func (broadcastEnvelopeCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(struct {
		Payload any `json:"payload"`
	}{Payload: value})
}

func (broadcastEnvelopeCodec) Unmarshal(data []byte, ptr any) error {
	var envelope struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Payload, ptr)
}

func broadcastContext(tb testing.TB) context.Context {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	tb.Cleanup(cancel)
	return ctx
}

// The Broadcast contract publishes after MakeSubscriber but before Subscribe.
func bindBroadcastSubscriber[T any](tb testing.TB, parent context.Context, source pubsub.Subscriber[T]) *broadcastBoundSubscriber[T] {
	tb.Helper()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	sub := source.Subscribe(ctx)
	var once sync.Once
	bound := &broadcastBoundSubscriber[T]{
		cancel: cancel,
		sub: func(yield func(pubsub.Message[T], error) bool) {
			once.Do(func() { sub(yield) })
		},
	}
	tb.Cleanup(func() {
		cancel()
		// Cancellation alone does not run a lazy LISTEN iterator's defers.
		// Enter it once if no consumer ever did; otherwise wait for its exit.
		bound.sub(func(pubsub.Message[T], error) bool { return false })
	})
	return bound
}

type broadcastBoundSubscriber[T any] struct {
	sub    pubsub.Subscription[T]
	cancel context.CancelFunc
}

func (s *broadcastBoundSubscriber[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	stop := context.AfterFunc(ctx, s.cancel)
	return func(yield func(pubsub.Message[T], error) bool) {
		defer stop()
		s.sub(yield)
	}
}

// Each binding starts without history. Prevent contract clean-ahead from
// draining (and thereby ending) the single-use subscription it is about to test.
func (*broadcastBoundSubscriber[T]) Purge(context.Context) error { return nil }

type broadcastSubscription[T any] struct {
	Next func() (pubsub.Message[T], error, bool)
}

func pullBroadcastSubscription[T any](tb testing.TB, ctx context.Context, source pubsub.Subscriber[T]) *broadcastSubscription[T] {
	tb.Helper()
	bound := bindBroadcastSubscriber(tb, ctx, source)
	next, stop := iter.Pull2(bound.Subscribe(ctx))
	tb.Cleanup(func() {
		bound.cancel()
		stop()
	})
	return &broadcastSubscription[T]{Next: next}
}

func (s *broadcastSubscription[T]) Receive(tb testing.TB) pubsub.Message[T] {
	tb.Helper()
	msg, err, ok := s.Next()
	assert.Must(tb).NoError(err)
	assert.Must(tb).True(ok, "expected a message before the test operation deadline")
	assert.Must(tb).NotNil(msg)
	return msg
}
