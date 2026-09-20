package postgresql_test

import (
	"context"
	"iter"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func workflowNotificationSpec(s *testcase.Spec, subject testcase.Var[*postgresql.WorkflowNotificationBroadcast], poolSize testcase.Var[int32]) {
	subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
		b := subject.Super(t)
		b.PollInterval = 10 * time.Millisecond
		return b
	})
	var (
		ctx   = let.Var(s, func(t *testcase.T) context.Context { return workflowNotificationContext(t) })
		batch = let.Var(s, func(t *testcase.T) []workflow.Notification {
			pid := wftest.MakeProcessID(t)
			return []workflow.Notification{
				workflow.ProcessSchedule{ProcessID: pid}, workflow.ProcessCancel{ProcessID: pid},
				workflow.ProcessSchedule{ProcessID: wftest.MakeProcessID(t)},
			}
		})
	)

	s.Describe("#Migrate", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Migrate(ctx.Get(t)) }
		s.Test("can initialize the schema repeatedly before any subscriptions", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			workflowNotificationRows(t, subject.Get(t), 0, 0)
		})
	})

	s.Describe("#Subscribe", func(s *testcase.Spec) {
		act := func(t *testcase.T) *workflowNotificationSubscription {
			return workflowNotificationSubscribe(t, ctx.Get(t), subject.Get(t))
		}

		s.Test("registers immediately, broadcasts the full ordered feed, and retains only the slowest subscriber's unread suffix", func(t *testcase.T) {
			fast, slow := act(t), act(t)
			b := subject.Get(t)
			workflowNotificationRows(t, b, 2, 0)
			for _, event := range batch.Get(t) {
				assert.Must(t).NoError(b.Publish(ctx.Get(t), event))
			}
			for _, event := range batch.Get(t) {
				msg := fast.Receive(t)
				assert.Equal(t, msg.Data(), event)
				assert.NoError(t, msg.NACK())
				assert.NoError(t, msg.ACK())
				assert.NoError(t, msg.NACK())
			}
			workflowNotificationRows(t, b, 2, 3)
			for i, event := range batch.Get(t) {
				assert.Equal(t, slow.Receive(t).Data(), event)
				// The handler is still paused at yield, without ACK or NACK.
				workflowNotificationRows(t, b, 2, len(batch.Get(t))-i-1)
			}
			fast.Stop()
			slow.Stop()
			workflowNotificationRows(t, b, 0, 0)
		})

		s.Test("keeps publishing with several polling readers and an idle subscriber on one connection", func(t *testcase.T) {
			idle, b := act(t), subject.Get(t)
			var readers []*pubsubtest.AsyncResults[workflow.Notification]
			for range 3 {
				readers = append(readers, pubsubtest.Subscribe(t, b, ctx.Get(t)))
			}
			assert.Eventually(t, time.Second, func(it testing.TB) {
				assert.Equal(it, b.Connection.DB.Stat().AcquiredConns(), int32(0))
			})
			for _, event := range batch.Get(t) {
				assert.Must(t).NoError(b.Publish(ctx.Get(t), event))
				assert.Equal(t, idle.Receive(t).Data(), event)
			}
			for _, reader := range readers {
				reader.Eventually(t, func(it testing.TB, got []workflow.Notification) {
					assert.Equal(it, got, batch.Get(t))
				})
			}
		})

		s.Test("joins at the current revision rather than replaying events retained for another subscriber", func(t *testcase.T) {
			old, b := act(t), subject.Get(t)
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
			newcomer := act(t)
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[1]))
			assert.Equal(t, newcomer.Receive(t).Data(), batch.Get(t)[1])
			workflowNotificationRows(t, b, 2, 2)
			assert.Equal(t, old.Receive(t).Data(), batch.Get(t)[0])
			assert.Equal(t, old.Receive(t).Data(), batch.Get(t)[1])
			workflowNotificationRows(t, b, 2, 0)
		})

		s.Test("cancels the last never-started subscription, cleans its feed, and reports the cancellation cause", func(t *testcase.T) {
			sub, b := act(t), subject.Get(t)
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
			cause := t.Random.Error()
			sub.Cancel(cause)
			assert.Eventually(t, time.Second, func(it testing.TB) { workflowNotificationRows(it, b, 0, 0) })
			msg, err, ok := sub.Next()
			assert.True(t, ok)
			assert.Nil(t, msg)
			assert.ErrorIs(t, err, cause)
		})

		s.Test("unregisters on an iteration break and rejects a repeated invocation explicitly", func(t *testcase.T) {
			sub, b := act(t), subject.Get(t)
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
			assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[0])
			sub.Stop()
			workflowNotificationRows(t, b, 0, 0)
			var repeated error
			sub.Iterator(func(msg pubsub.Message[workflow.Notification], err error) bool {
				assert.Nil(t, msg)
				repeated = err
				return false
			})
			assert.Error(t, repeated)
		})

		s.Test("isolates channels including cancellation and cleanup", func(t *testcase.T) {
			sub, b := act(t), subject.Get(t)
			other := &postgresql.WorkflowNotificationBroadcast{Connection: b.Connection, Name: b.Name + "_other", StatelessSubscribe: true}
			otherSub := workflowNotificationSubscribe(t, ctx.Get(t), other)
			assert.Must(t).NoError(other.Publish(ctx.Get(t), batch.Get(t)[0]))
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[1]))
			workflowNotificationRows(t, b, 1, 1)
			workflowNotificationRows(t, other, 1, 1)
			sub.Cancel(context.Canceled)
			assert.Eventually(t, time.Second, func(it testing.TB) { workflowNotificationRows(it, b, 0, 0) })
			assert.Equal(t, otherSub.Receive(t).Data(), batch.Get(t)[0])
			assert.Must(t).NoError(other.Publish(ctx.Get(t), batch.Get(t)[2]))
			assert.Equal(t, otherSub.Receive(t).Data(), batch.Get(t)[2])
		})

		s.When("Name needs normalization", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				b := subject.Super(t)
				b.Name = " 1-" + strings.ToUpper(b.Name) + ".feed "
				return b
			})
			s.Then("uses the normalized channel for registration and feed storage", func(t *testcase.T) {
				sub, b := act(t), subject.Get(t)
				workflowNotificationRows(t, b, 1, 0)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
				workflowNotificationRows(t, b, 1, 1)
				assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[0])
			})
		})

		s.When("another pool uses both subscription modes", func(s *testcase.Spec) {
			poolSize.LetValue(s, 4)
			s.Then("fans out publications from both modes across instances and pools", func(t *testcase.T) {
				local, b := act(t), subject.Get(t)
				remote := queueV2ConnectPool(t, b.Connection.DB.Config())
				listen := &postgresql.WorkflowNotificationBroadcast{Connection: remote, Name: b.Name}
				poll := &postgresql.WorkflowNotificationBroadcast{Connection: remote, Name: b.Name, StatelessSubscribe: true}
				subs := []*workflowNotificationSubscription{local,
					workflowNotificationSubscribe(t, ctx.Get(t), listen), workflowNotificationSubscribe(t, ctx.Get(t), poll)}
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
				assert.Must(t).NoError(listen.Publish(ctx.Get(t), batch.Get(t)[1]))
				for _, sub := range subs {
					assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[0])
					assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[1])
				}
			})
		})

		s.When("the lease duration is short", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				b := subject.Super(t)
				b.SubscriberLeaseDuration = 250 * time.Millisecond
				return b
			})
			s.Then("renews an uniterated subscriber and preserves unread events beyond its original lease", func(t *testcase.T) {
				sub, b := act(t), subject.Get(t)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
				time.Sleep(750 * time.Millisecond)
				workflowNotificationRows(t, b, 1, 1)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[1]))
				assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[0])
				assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[1])
			})
			s.Then("renews a paused handler and cleans up canceled subscribers without waiting for that handler", func(t *testcase.T) {
				slow, fast, b := act(t), act(t), subject.Get(t)
				for _, event := range batch.Get(t)[:2] {
					assert.Must(t).NoError(b.Publish(ctx.Get(t), event))
				}
				assert.Equal(t, fast.Receive(t).Data(), batch.Get(t)[0])
				time.Sleep(750 * time.Millisecond)
				slow.Cancel(context.Canceled)
				assert.Eventually(t, time.Second, func(it testing.TB) { workflowNotificationRows(it, b, 1, 1) })
				assert.Equal(t, fast.Receive(t).Data(), batch.Get(t)[1])
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[2]))
				fast.Cancel(context.Canceled)
				assert.Eventually(t, time.Second, func(it testing.TB) { workflowNotificationRows(it, b, 0, 0) })
			})
			s.Then("expires after pool starvation, prunes abandoned data on publish, and recovers without replay", func(t *testcase.T) {
				sub, b := act(t), subject.Get(t)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
				publisher := &postgresql.WorkflowNotificationBroadcast{
					Connection: queueV2ConnectPool(t, b.Connection.DB.Config()), Name: b.Name,
				}
				release := queueV2StarvePool(t, b.Connection.DB)
				time.Sleep(750 * time.Millisecond)
				// Observe and prune from another pool while the subscriber cannot perform cleanup.
				workflowNotificationRows(t, publisher, 1, 1)
				assert.Must(t).NoError(publisher.Publish(ctx.Get(t), batch.Get(t)[1]))
				workflowNotificationRows(t, publisher, 0, 0)
				release()
				msg, err, ok := sub.Next()
				assert.True(t, ok)
				assert.Nil(t, msg)
				assert.ErrorIs(t, err, postgresql.ErrSubscriptionExpired)
				recovered := act(t)
				assert.Must(t).NoError(publisher.Publish(ctx.Get(t), batch.Get(t)[2]))
				assert.Equal(t, recovered.Receive(t).Data(), batch.Get(t)[2])
			})
		})

		for _, tc := range []struct {
			name                  string
			configured, effective time.Duration
		}{{"default lease", 0, 30 * time.Second}, {"minimum lease", 100 * time.Millisecond, 100 * time.Millisecond}} {
			s.When(tc.name, func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
					b := subject.Super(t)
					b.SubscriberLeaseDuration = tc.configured
					return b
				})
				s.Then("registers with the effective lease duration", func(t *testcase.T) {
					b := subject.Get(t)
					var before, expires, now time.Time
					assert.Must(t).NoError(b.Connection.DB.QueryRow(ctx.Get(t), `SELECT clock_timestamp()`).Scan(&before))
					_ = act(t)
					assert.Must(t).NoError(b.Connection.DB.QueryRow(ctx.Get(t), `SELECT expires_at, clock_timestamp()
						FROM frameless_workflow_notification_subscribers WHERE channel = $1`, workflowNotificationChannel(b.Name)).Scan(&expires, &now))
					assert.False(t, expires.Before(before.Add(tc.effective)))
					assert.False(t, expires.After(now.Add(tc.effective+10*time.Millisecond)))
				})
			})
		}

		s.When("the lease duration is below the minimum", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				b := subject.Super(t)
				b.SubscriberLeaseDuration = time.Millisecond
				return b
			})
			s.Before(func(t *testcase.T) { assert.Must(t).NoError(subject.Get(t).Migrate(ctx.Get(t))) })
			s.Then("rejects the subscription without registering or retaining data", func(t *testcase.T) {
				sub, b := act(t), subject.Get(t)
				workflowNotificationRows(t, b, 0, 0)
				msg, err, ok := sub.Next()
				assert.True(t, ok)
				assert.Nil(t, msg)
				assert.Error(t, err)
				assert.NoError(t, ctx.Get(t).Err(), "configuration errors must not wait for cancellation")
				_, _, ok = sub.Next()
				assert.False(t, ok)
				workflowNotificationRows(t, b, 0, 0)
			})
		})

		s.Test("reports a vanished registration as expired rather than silently subscribing again", func(t *testcase.T) {
			sub, b := act(t), subject.Get(t)
			_, err := b.Connection.DB.Exec(ctx.Get(t), `DELETE FROM frameless_workflow_notification_subscribers WHERE channel = $1`, workflowNotificationChannel(b.Name))
			assert.Must(t).NoError(err)
			msg, err, ok := sub.Next()
			assert.True(t, ok)
			assert.Nil(t, msg)
			assert.ErrorIs(t, err, postgresql.ErrSubscriptionExpired)
		})

		s.When("the injected codec cannot decode", func(s *testcase.Spec) {
			decodeError := let.Error(s)
			subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				b := subject.Super(t)
				b.Codec = workflowNotificationFailingCodec{Codec: wfjson.NewCodec(), Err: decodeError.Get(t)}
				return b
			})
			s.Then("reports the decoding error, terminates, and unregisters", func(t *testcase.T) {
				sub, b := act(t), subject.Get(t)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[0]))
				msg, err, ok := sub.Next()
				assert.True(t, ok)
				assert.Nil(t, msg)
				assert.ErrorIs(t, err, decodeError.Get(t))
				_, _, ok = sub.Next()
				assert.False(t, ok)
				workflowNotificationRows(t, b, 0, 0)
			})
		})
	})

	s.Describe("#Publish", func(s *testcase.Spec) {
		var (
			publishCtx = let.Var(s, func(t *testcase.T) context.Context { return ctx.Get(t) })
			event      = let.Var(s, func(t *testcase.T) workflow.Notification { return batch.Get(t)[0] })
		)
		act := func(t *testcase.T) error { return subject.Get(t).Publish(publishCtx.Get(t), event.Get(t)) }

		s.Test("initializes lazily and stores no data without subscribers or replays for later subscribers", func(t *testcase.T) {
			assert.Must(t).NoError(act(t))
			b := subject.Get(t)
			workflowNotificationRows(t, b, 0, 0)
			sub := workflowNotificationSubscribe(t, ctx.Get(t), b)
			assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[1]))
			assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[1])
		})

		s.When("a custom notification codec is injected", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
				b := subject.Super(t)
				c := wfjson.NewCodec()
				jsonkit.CodecRegisterTypeID[workflowNotificationData](c, "test::notification")
				b.Codec = c
				return b
			})
			event.Let(s, func(t *testcase.T) workflow.Notification {
				return workflowNotificationData{ProcessID: batch.Get(t)[0].GetProcessID(), Text: t.Random.UUID()}
			})
			s.Then("persists and decodes the injected wire format", func(t *testcase.T) {
				b := subject.Get(t)
				sub := workflowNotificationSubscribe(t, ctx.Get(t), b)
				assert.Must(t).NoError(act(t))
				var data []byte
				assert.Must(t).NoError(b.Connection.DB.QueryRow(ctx.Get(t), `SELECT data FROM frameless_workflow_notification_events WHERE channel = $1`, workflowNotificationChannel(b.Name)).Scan(&data))
				want, err := b.Codec.Marshal(event.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, data, want)
				assert.Equal(t, sub.Receive(t).Data(), event.Get(t))
			})
			s.And("the encoded notification exceeds the pg_notify payload limit", func(s *testcase.Spec) {
				event.Let(s, func(t *testcase.T) workflow.Notification {
					v := event.Super(t).(workflowNotificationData)
					v.Text = strings.Repeat("x", 8100)
					return v
				})
				s.Then("rejects the publish without leaving a feed event", func(t *testcase.T) {
					b := subject.Get(t)
					_ = workflowNotificationSubscribe(t, ctx.Get(t), b)
					data, err := b.Codec.Marshal(event.Get(t))
					assert.Must(t).NoError(err)
					assert.Must(t).True(len(data) > 8000, "the fixture must exceed the pg_notify payload limit")
					err = act(t)
					assert.Must(t).Error(err)
					assert.Contains(t, err.Error(), "payload")
					workflowNotificationRows(t, b, 1, 0)
				})
			})
		})

		s.When("publishing inside a caller-owned transaction", func(s *testcase.Spec) {
			poolSize.LetValue(s, 4)
			s.Before(func(t *testcase.T) { assert.Must(t).NoError(subject.Get(t).Migrate(ctx.Get(t))) })
			publishCtx.Let(s, func(t *testcase.T) context.Context {
				b := subject.Get(t)
				tx, err := b.Connection.BeginTx(ctx.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = b.Connection.RollbackTx(tx) })
				return tx
			})
			s.Then("makes the feed and LISTEN notification visible only after commit", func(t *testcase.T) {
				b := subject.Get(t)
				sub := workflowNotificationSubscribe(t, ctx.Get(t), b)
				listen := pubsubtest.Subscribe(t, &postgresql.WorkflowNotificationBroadcast{Connection: b.Connection, Name: b.Name}, ctx.Get(t))
				assert.Must(t).NoError(act(t))
				var count int
				assert.Must(t).NoError(b.Connection.QueryRowContext(publishCtx.Get(t), `SELECT count(*) FROM frameless_workflow_notification_events WHERE channel = $1`, workflowNotificationChannel(b.Name)).Scan(&count))
				assert.Equal(t, count, 1)
				workflowNotificationRows(t, b, 1, 0)
				time.Sleep(100 * time.Millisecond)
				assert.Empty(t, listen.Values())
				assert.Must(t).NoError(b.Connection.CommitTx(publishCtx.Get(t)))
				assert.Equal(t, sub.Receive(t).Data(), event.Get(t))
				listen.Eventually(t, func(it testing.TB, got []workflow.Notification) {
					assert.Equal(it, got, []workflow.Notification{event.Get(t)})
				})
			})
			s.Then("serializes concurrent publishers in commit order before handing off either notification", func(t *testcase.T) {
				b := subject.Get(t)
				sub := workflowNotificationSubscribe(t, ctx.Get(t), b)
				cfg := b.Connection.DB.Config()
				cfg.MaxConns = 1
				other := &postgresql.WorkflowNotificationBroadcast{Connection: queueV2ConnectPool(t, cfg), Name: b.Name}
				assert.Must(t).NoError(other.Migrate(ctx.Get(t)))
				var firstPID, secondPID int
				assert.Must(t).NoError(other.Connection.DB.QueryRow(ctx.Get(t), `SELECT pg_backend_pid()`).Scan(&secondPID))
				assert.Must(t).NoError(act(t))
				assert.Must(t).NoError(b.Connection.QueryRowContext(publishCtx.Get(t), `SELECT pg_backend_pid()`).Scan(&firstPID))

				secondCtx, cancel := context.WithCancel(ctx.Get(t))
				secondEvent := batch.Get(t)[1]
				done := make(chan struct{})
				var publishErr error
				go func() {
					defer close(done)
					publishErr = other.Publish(secondCtx, secondEvent)
				}()
				t.Cleanup(func() {
					cancel()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("concurrent publisher did not stop after cancellation")
					}
				})
				// Observe the actual lock wait, not merely that the goroutine was scheduled.
				assert.Eventually(t, time.Second, func(it testing.TB) {
					var blocked bool
					assert.NoError(it, b.Connection.DB.QueryRow(ctx.Get(t),
						`SELECT $1::integer = ANY(pg_blocking_pids($2::integer))`, firstPID, secondPID).Scan(&blocked))
					assert.True(it, blocked, "the first transaction must block the second publisher")
				})
				select {
				case <-done:
					t.Fatalf("second Publish returned before the first transaction committed: %v", publishErr)
				case <-time.After(100 * time.Millisecond):
				}
				assert.Must(t).NoError(b.Connection.CommitTx(publishCtx.Get(t)))
				select {
				case <-done:
					assert.Must(t).NoError(publishErr)
				case <-ctx.Get(t).Done():
					t.Fatal("second Publish did not complete after the first transaction committed")
				}
				workflowNotificationRows(t, b, 1, 2)
				assert.Equal(t, sub.Receive(t).Data(), event.Get(t))
				assert.Equal(t, sub.Receive(t).Data(), secondEvent)
			})
			s.Then("rolls back both feed and LISTEN delivery without poisoning subsequent publications", func(t *testcase.T) {
				b := subject.Get(t)
				sub := workflowNotificationSubscribe(t, ctx.Get(t), b)
				listen := pubsubtest.Subscribe(t, &postgresql.WorkflowNotificationBroadcast{Connection: b.Connection, Name: b.Name}, ctx.Get(t))
				assert.Must(t).NoError(act(t))
				assert.Must(t).NoError(b.Connection.RollbackTx(publishCtx.Get(t)))
				workflowNotificationRows(t, b, 1, 0)
				assert.Must(t).NoError(b.Publish(ctx.Get(t), batch.Get(t)[1]))
				assert.Equal(t, sub.Receive(t).Data(), batch.Get(t)[1])
				listen.Eventually(t, func(it testing.TB, got []workflow.Notification) {
					assert.Equal(it, got, []workflow.Notification{batch.Get(t)[1]})
				})
			})
		})

		s.When("another instance migrated storage and the pool has only one connection", func(s *testcase.Spec) {
			poolSize.LetValue(s, 1)
			subscriber := let.Var(s, func(t *testcase.T) *workflowNotificationSubscription {
				b := subject.Get(t)
				migrator := &postgresql.WorkflowNotificationBroadcast{Connection: b.Connection, Name: b.Name, StatelessSubscribe: true}
				assert.Must(t).NoError(migrator.Migrate(ctx.Get(t)))
				return workflowNotificationSubscribe(t, ctx.Get(t), migrator)
			})
			s.Before(func(t *testcase.T) { subscriber.Get(t) })
			publishCtx.Let(s, func(t *testcase.T) context.Context {
				b := subject.Get(t)
				tx, err := b.Connection.BeginTx(ctx.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = b.Connection.RollbackTx(tx) })
				return tx
			})
			s.Then("allows a fresh publisher to use the caller's transaction without acquiring another connection", func(t *testcase.T) {
				b := subject.Get(t)
				assert.Equal(t, b.Connection.DB.Config().MaxConns, int32(1))
				assert.Must(t).NoError(act(t))
				var count int
				assert.Must(t).NoError(b.Connection.QueryRowContext(publishCtx.Get(t),
					`SELECT count(*) FROM frameless_workflow_notification_events WHERE channel = $1`, workflowNotificationChannel(b.Name)).Scan(&count))
				assert.Equal(t, count, 1)
				assert.Must(t).NoError(b.Connection.CommitTx(publishCtx.Get(t)))
				assert.Equal(t, subscriber.Get(t).Receive(t).Data(), event.Get(t))
				workflowNotificationRows(t, b, 1, 0)
			})
		})
	})
}

func workflowNotificationContext(tb testing.TB) context.Context {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	tb.Cleanup(cancel)
	return ctx
}

type workflowNotificationSubscription struct {
	Iterator pubsub.Subscription[workflow.Notification]
	Next     func() (pubsub.Message[workflow.Notification], error, bool)
	Stop     func()
	Cancel   context.CancelCauseFunc
}

func workflowNotificationSubscribe(tb testing.TB, parent context.Context, b *postgresql.WorkflowNotificationBroadcast) *workflowNotificationSubscription {
	tb.Helper()
	ctx, cancel := context.WithCancelCause(parent)
	sub := b.Subscribe(ctx)
	next, stop := iter.Pull2(sub)
	tb.Cleanup(func() { cancel(context.Canceled); stop() })
	return &workflowNotificationSubscription{Iterator: sub, Next: next, Stop: stop, Cancel: cancel}
}

func (sub *workflowNotificationSubscription) Receive(tb testing.TB) pubsub.Message[workflow.Notification] {
	tb.Helper()
	msg, err, ok := sub.Next()
	assert.Must(tb).NoError(err)
	assert.Must(tb).True(ok, "expected a notification before the operation deadline")
	assert.Must(tb).NotNil(msg)
	return msg
}

func workflowNotificationRows(tb testing.TB, b *postgresql.WorkflowNotificationBroadcast, subscribers, events int) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var gotSubscribers, gotEvents int
	err := b.Connection.DB.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM frameless_workflow_notification_subscribers WHERE channel = $1),
		(SELECT count(*) FROM frameless_workflow_notification_events WHERE channel = $1)`, workflowNotificationChannel(b.Name)).Scan(&gotSubscribers, &gotEvents)
	assert.Must(tb).NoError(err)
	assert.Equal(tb, gotSubscribers, subscribers, "registered subscribers")
	assert.Equal(tb, gotEvents, events, "retained notifications")
}

func workflowNotificationChannel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "frameless_workflow_notifications"
	}
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, name)
	if name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	return name
}

type workflowNotificationData struct {
	ProcessID workflow.ProcessID
	Text      string
}

func (n workflowNotificationData) GetProcessID() workflow.ProcessID { return n.ProcessID }
func (workflowNotificationData) NotificationType() workflow.NotificationType {
	return "test::notification"
}

type workflowNotificationFailingCodec struct {
	workflow.Codec
	Err error
}

func (c workflowNotificationFailingCodec) Unmarshal([]byte, any) error { return c.Err }
