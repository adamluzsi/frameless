package postgresql_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestQueue_v2(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		makeQueue = let.Var(s, func(t *testcase.T) func(queueV2Config) postgresql.Queue[Entity] {
			return func(cfg queueV2Config) postgresql.Queue[Entity] {
				return postgresql.Queue[Entity]{
					Table: cfg.Name, Connection: cfg.Connection,
					Codec: cfg.Codec, ToMeta: cfg.ToMeta,
					LIFO: cfg.LIFO, SortBy: cfg.SortBy, Blocking: cfg.Blocking,
					EmptyBreakTime:              cfg.EmptyQueueBreakTime,
					FencingLeaseDuration:        cfg.OwnershipDuration,
					TransactionalMessageContext: cfg.TransactionalMessageContext,
				}
			}
		})
		poolSize   = let.Var(s, func(t *testcase.T) int32 { return 2 })
		connection = let.Var(s, func(t *testcase.T) postgresql.Connection {
			return queueV2Connection(t, poolSize.Get(t))
		})
		name              = let.Var(s, func(t *testcase.T) string { return "messages" })
		ownershipDuration = let.Var(s, func(t *testcase.T) time.Duration { return 0 })
		callerValue       = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() })
		callerContext     = let.Var(s, func(t *testcase.T) context.Context {
			return context.WithValue(queueV2Context(t), queueV2ContextKey{}, callerValue.Get(t))
		})
		config = let.Var(s, func(t *testcase.T) queueV2Config {
			return queueV2Config{
				Name: name.Get(t), Connection: connection.Get(t),
				EmptyQueueBreakTime: 10 * time.Millisecond,
				OwnershipDuration:   ownershipDuration.Get(t),
			}
		})
		subject = let.Var(s, func(t *testcase.T) postgresql.Queue[Entity] {
			return makeQueue.Get(t)(config.Get(t))
		})
		ready = let.Var(s, func(t *testcase.T) postgresql.Queue[Entity] {
			q := subject.Get(t)
			m := q.Migrator()
			// Migration uses separate transactions for state and steps. Keep the extra
			// pool in the queue's schema without increasing its runtime pool size.
			m.Resource = queueV2ConnectPool(t, q.Connection.DB.Config())
			assert.NoError(t, m.Migrate(queueV2Context(t)))
			return q
		})
		other = let.Var(s, func(t *testcase.T) postgresql.Queue[Entity] {
			make := makeQueue.Get(t)
			cfg := config.Get(t)
			cfg.Name += "_other"
			q := make(cfg)
			m := q.Migrator()
			m.Resource = queueV2ConnectPool(t, q.Connection.DB.Config())
			assert.NoError(t, m.Migrate(queueV2Context(t)))
			return q
		})
		data = let.Var(s, func(t *testcase.T) Entity {
			return Entity{ID: t.Random.UUID(), Foo: t.Random.String(), Bar: t.Random.String(), Baz: t.Random.String()}
		})
		batch = let.Var(s, func(t *testcase.T) []Entity {
			return []Entity{
				{ID: t.Random.UUID(), Foo: t.Random.String(), Bar: "2"},
				{ID: t.Random.UUID(), Foo: t.Random.String(), Bar: "1"},
				{ID: t.Random.UUID(), Foo: t.Random.String(), Bar: "3"},
			}
		})
		deliverySub = let.Var(s, func(t *testcase.T) *queueV2Subscription {
			q := ready.Get(t)
			assert.NoError(t, q.Publish(queueV2Context(t), data.Get(t)))
			return queueV2SubscribeContext(t, q, callerContext.Get(t))
		})
		delivery = let.Var(s, func(t *testcase.T) pubsub.Message[Entity] {
			return deliverySub.Get(t).Receive(t)
		})
		replacement = let.Var(s, func(t *testcase.T) pubsub.Message[Entity] {
			return queueV2Subscribe(t, ready.Get(t)).Receive(t)
		})
	)

	s.Test("pubsub contracts", func(t *testcase.T) {
		basicQueue := ready.Get(t)

		lifoConfig := config.Get(t)
		lifoConfig.LIFO = true
		lifoQueue := makeQueue.Get(t)(lifoConfig)

		blockingConfig := config.Get(t)
		blockingConfig.Blocking = true
		blockingQueue := makeQueue.Get(t)(blockingConfig)
		assert.NoError(t, blockingQueue.Migrate(queueV2Context(t)))

		transactionalConfig := config.Get(t)
		transactionalConfig.TransactionalMessageContext = true
		transactionalQueue := makeQueue.Get(t)(transactionalConfig)

		// Keep the contract groups under this subtest so they can be run independently.
		testcase.RunSuite(testcase.NewSpec(t.TB),
			pubsubcontract.FIFO[Entity](basicQueue, basicQueue),
			pubsubcontract.LIFO[Entity](lifoQueue, lifoQueue),
			pubsubcontract.Durable[Entity](basicQueue, basicQueue),
			pubsubcontract.Blocking[Entity](blockingQueue, blockingQueue),
			pubsubcontract.Queue[Entity](basicQueue, basicQueue),
			pubsubcontract.NonTransactionalMessageContext[Entity](basicQueue, basicQueue),
			pubsubcontract.TransactionalMessageContext[Entity](basicQueue, transactionalQueue),
			pubsubcontract.TransactionalPublisher[Entity](basicQueue, basicQueue, connection.Get(t)),
		)
	})

	s.Describe("#Publish", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return ready.Get(t).Publish(queueV2Context(t), data.Get(t))
		}

		s.Test("preserves the published payload", func(t *testcase.T) {
			assert.NoError(t, act(t))
			msg := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
			queueV2AssertEmpty(t, ready.Get(t))
		})

		s.When("a non-JSON codec is configured", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.Codec = queueV2GobCodec{}
				return cfg
			})

			decodeMismatch := let.Var(s, func(t *testcase.T) error { return errors.New(t.Random.String()) })

			s.Then("round-trips the payload using that codec", func(t *testcase.T) {
				assert.NoError(t, act(t))
				msg := queueV2Subscribe(t, ready.Get(t)).Receive(t)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.ACK())
			})

			s.Then("stores codec-specific bytes rather than silently falling back to JSON", func(t *testcase.T) {
				assert.NoError(t, act(t))
				cfg := config.Get(t)
				mismatch := decodeMismatch.Get(t)
				cfg.Codec = struct {
					codec.Marshaler
					codec.Unmarshaler
				}{jsonkit.Codec{}, codec.UnmarshalerFunc(func(data []byte, ptr any) error {
					if err := (jsonkit.Codec{}).Unmarshal(data, ptr); err != nil {
						return errors.Join(mismatch, err)
					}
					return nil
				})}
				reader := makeQueue.Get(t)(cfg)
				_, err, ok := queueV2Subscribe(t, reader).Next()
				assert.True(t, ok)
				assert.ErrorIs(t, err, mismatch, "JSON must not decode a gob-encoded payload")
			})
		})
	})

	s.Describe("blocking #Publish", func(s *testcase.Spec) {
		act := func(t *testcase.T) <-chan error {
			ready.Get(t)
			cfg := config.Get(t)
			cfg.Blocking = true
			publisher := makeQueue.Get(t)(cfg)
			assert.NoError(t, publisher.Migrate(queueV2Context(t)))
			return queueV2PublishAsync(t, publisher, data.Get(t))
		}

		s.Test("waits for acknowledgement rather than merely claiming the message", func(t *testcase.T) {
			result := act(t)
			msg := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, msg.Data(), data.Get(t))
			queueV2AssertPublishing(t, result)
			assert.NoError(t, msg.ACK())
			assert.NoError(t, queueV2AwaitPublished(t, result))
		})

		s.Test("an existing nonblocking subscription acknowledges later blocking publications", func(t *testcase.T) {
			q := ready.Get(t)
			assert.NoError(t, q.Publish(queueV2Context(t), batch.Get(t)[0]))
			sub := queueV2Subscribe(t, q)
			first := sub.Receive(t)
			assert.Equal(t, first.Data(), batch.Get(t)[0])
			assert.NoError(t, first.ACK())

			result := act(t)
			msg := sub.Receive(t)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
			assert.NoError(t, queueV2AwaitPublished(t, result))
		})

		s.Test("does not mistake negative acknowledgement or redelivery for successful processing", func(t *testcase.T) {
			result := act(t)
			msg := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.NoError(t, msg.NACK())
			replacement := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, replacement.Data(), data.Get(t))
			queueV2AssertPublishing(t, result)
			assert.NoError(t, replacement.ACK())
			assert.NoError(t, queueV2AwaitPublished(t, result))
		})

		s.Test("abandonment does not complete the publisher until a replacement acknowledges", func(t *testcase.T) {
			result := act(t)
			sub := queueV2Subscribe(t, ready.Get(t))
			assert.Equal(t, sub.Receive(t).Data(), data.Get(t))
			sub.Stop()
			queueV2AssertPublishing(t, result)
			recovered := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, recovered.Data(), data.Get(t))
			assert.NoError(t, recovered.ACK())
			assert.NoError(t, queueV2AwaitPublished(t, result))
		})

		s.Test("purging a waiting message returns ErrQueueMessagePurged rather than successful processing", func(t *testcase.T) {
			result := act(t)
			assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
				assert.Equal(it, queueV2MessageCount(it, connection.Get(t), name.Get(t)), 1)
			})
			queueV2AssertPublishing(t, result)
			assert.NoError(t, ready.Get(t).Purge(queueV2Context(t)))
			assert.ErrorIs(t, queueV2AwaitPublished(t, result), postgresql.ErrQueueMessagePurged)
			queueV2AssertEmpty(t, ready.Get(t))
		})

		s.Test("purging an active message neither revokes its owner nor completes its publisher", func(t *testcase.T) {
			result := act(t)
			msg := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.NoError(t, ready.Get(t).Purge(queueV2Context(t)))
			queueV2AssertPublishing(t, result)
			assert.NoError(t, msg.Context().Err())
			assert.NoError(t, msg.ACK())
			assert.NoError(t, queueV2AwaitPublished(t, result))
		})

		s.When("the nonblocking consumer uses transactional message contexts", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.TransactionalMessageContext = true
				return cfg
			})

			s.Then("ACK commits the receipt and completes the blocking publisher", func(t *testcase.T) {
				result := act(t)
				consumer := ready.Get(t)
				assert.False(t, consumer.Blocking)
				msg := queueV2Subscribe(t, consumer).Receive(t)
				assert.Equal(t, msg.Data(), data.Get(t))
				_, transactional := connection.Get(t).LookupTx(msg.Context())
				assert.True(t, transactional)
				queueV2AssertPublishing(t, result)
				assert.NoError(t, msg.ACK())
				assert.NoError(t, queueV2AwaitPublished(t, result))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
			})
		})
	})

	s.Describe("#Subscribe selection", func(s *testcase.Spec) {
		var expected = let.Var(s, func(t *testcase.T) []Entity { return batch.Get(t) })
		act := func(t *testcase.T) *queueV2Subscription {
			return queueV2Subscribe(t, ready.Get(t))
		}

		s.Before(func(t *testcase.T) {
			q := ready.Get(t)
			// Separate publishes establish FIFO/LIFO order without relying on batch timestamp ties.
			for _, value := range batch.Get(t) {
				assert.NoError(t, q.Publish(queueV2Context(t), value))
			}
		})

		s.Test("selects eligible messages in FIFO order by default", func(t *testcase.T) {
			sub := act(t)
			for _, value := range expected.Get(t) {
				msg := sub.Receive(t)
				assert.Equal(t, msg.Data(), value)
				assert.NoError(t, msg.ACK())
			}
		})

		s.Test("NACK preserves the original FIFO position rather than republishing at the end", func(t *testcase.T) {
			first := act(t).Receive(t)
			assert.NoError(t, first.NACK())
			sub := act(t)
			for _, value := range expected.Get(t) {
				msg := sub.Receive(t)
				assert.Equal(t, msg.Data(), value)
				assert.NoError(t, msg.ACK())
			}
		})

		s.Test("skips an owned message without forcing completion in delivery order", func(t *testcase.T) {
			first := act(t).Receive(t)
			second := act(t).Receive(t)
			assert.Equal(t, first.Data(), batch.Get(t)[0])
			assert.Equal(t, second.Data(), batch.Get(t)[1])
			assert.NoError(t, second.ACK())
			assert.NoError(t, first.Context().Err())
			assert.NoError(t, first.ACK())
		})

		s.When("LIFO is enabled", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.LIFO = true
				return cfg
			})
			expected.Let(s, func(t *testcase.T) []Entity {
				values := slices.Clone(batch.Get(t))
				slices.Reverse(values)
				return values
			})

			s.Then("selects eligible messages in LIFO order", func(t *testcase.T) {
				sub := act(t)
				for _, value := range expected.Get(t) {
					msg := sub.Receive(t)
					assert.Equal(t, msg.Data(), value)
					assert.NoError(t, msg.ACK())
				}
			})

			s.Then("NACK preserves the original LIFO position", func(t *testcase.T) {
				first := act(t).Receive(t)
				assert.NoError(t, first.NACK())
				sub := act(t)
				for _, value := range expected.Get(t) {
					msg := sub.Receive(t)
					assert.Equal(t, msg.Data(), value)
					assert.NoError(t, msg.ACK())
				}
			})
		})

		s.When("SortBy uses metadata with a non-JSON payload codec", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.Codec = queueV2GobCodec{}
				cfg.ToMeta = func(e Entity) map[string]any { return map[string]any{"priority": e.Bar} }
				cfg.SortBy = "(meta->>'priority')::integer ASC, created_at ASC"
				return cfg
			})
			expected.Let(s, func(t *testcase.T) []Entity {
				values := batch.Get(t)
				return []Entity{values[1], values[0], values[2]}
			})

			s.Then("selects by metadata independently of the payload encoding", func(t *testcase.T) {
				sub := act(t)
				for _, value := range expected.Get(t) {
					msg := sub.Receive(t)
					assert.Equal(t, msg.Data(), value)
					assert.NoError(t, msg.ACK())
				}
			})

			s.Then("NACK preserves the payload and its selection priority", func(t *testcase.T) {
				first := act(t).Receive(t)
				assert.Equal(t, first.Data(), expected.Get(t)[0])
				assert.NoError(t, first.NACK())
				replacement := act(t).Receive(t)
				assert.Equal(t, replacement.Data(), first.Data())
				assert.NoError(t, replacement.ACK())
			})
		})
	})

	s.Describe("concurrent processing", func(s *testcase.Spec) {
		act := func(t *testcase.T) []pubsub.Message[Entity] {
			var messages []pubsub.Message[Entity]
			for range batch.Get(t) {
				messages = append(messages, queueV2Subscribe(t, ready.Get(t)).Receive(t))
			}
			return messages
		}

		s.Before(func(t *testcase.T) {
			q := ready.Get(t)
			for _, value := range batch.Get(t) {
				assert.NoError(t, q.Publish(queueV2Context(t), value))
			}
		})

		s.Test("keeps deliveries exclusive while other available messages are consumed", func(t *testcase.T) {
			messages := act(t)
			var got []Entity
			for _, msg := range messages {
				assert.NoError(t, msg.Context().Err())
				got = append(got, msg.Data())
			}
			assert.ContainsExactly(t, got, batch.Get(t))
			queueV2AssertEmpty(t, ready.Get(t))
			for _, msg := range messages {
				assert.NoError(t, msg.ACK())
			}
		})

		s.When("more handlers than pool connections are holding unsettled deliveries", func(s *testcase.Spec) {
			poolSize.LetValue(s, int32(1))

			s.Then("delivers all available messages and leaves the pool usable before any handler finishes", func(t *testcase.T) {
				pool := ready.Get(t).Connection.DB
				assert.Must(t).Equal(pool.Stat().MaxConns(), int32(1))
				messages := act(t)
				assert.True(t, len(messages) > int(pool.Stat().MaxConns()))
				var got []Entity
				for _, msg := range messages {
					assert.NoError(t, msg.Context().Err())
					got = append(got, msg.Data())
				}
				assert.ContainsExactly(t, got, batch.Get(t))
				assert.NoError(t, pool.Ping(queueV2Context(t)))
				assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
					assert.Equal(it, pool.Stat().AcquiredConns(), int32(0))
				})
				for _, msg := range messages {
					assert.NoError(t, msg.ACK())
				}
			})
		})

	})

	s.Describe("simultaneous claims from independent pools", func(s *testcase.Spec) {
		type claimed struct {
			message pubsub.Message[Entity]
			err     error
		}
		act := func(t *testcase.T) []pubsub.Message[Entity] {
			var consumers []queueV2Subject
			for range batch.Get(t) {
				// Config returns a copy, retaining the primary pool's database and quoted search_path.
				poolConfig := connection.Get(t).DB.Config()
				poolConfig.MaxConns = 1
				cfg := config.Get(t)
				cfg.Connection = queueV2ConnectPool(t, poolConfig)
				assert.NoError(t, cfg.Connection.DB.Ping(queueV2Context(t)))
				consumers = append(consumers, makeQueue.Get(t)(cfg))
			}

			ctx, cancel := context.WithTimeout(context.Background(), queueV2OperationBudget)
			start := make(chan struct{})
			results := make(chan claimed, len(consumers))
			var waiting, workers sync.WaitGroup
			waiting.Add(len(consumers))
			workers.Add(len(consumers))
			for _, consumer := range consumers {
				go func() {
					defer workers.Done()
					waiting.Done()
					<-start
					for msg, err := range consumer.Subscribe(ctx) {
						results <- claimed{message: msg, err: err}
						if err == nil {
							// Do not advance, stop, or ACK until the test has observed every claim.
							<-ctx.Done()
						}
						return
					}
					results <- claimed{err: ctx.Err()}
				}()
			}
			done := make(chan struct{})
			go func() {
				workers.Wait()
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(queueV2OperationBudget):
					t.Error("independent claim subscriptions did not finish after cancellation")
				}
			})
			waiting.Wait()
			close(start)

			var messages []pubsub.Message[Entity]
			for range consumers {
				select {
				case result := <-results:
					assert.NoError(t, result.err)
					assert.Must(t).NotNil(result.message)
					messages = append(messages, result.message)
				case <-ctx.Done():
					t.Fatal("independent claim attempts did not all deliver before the operation deadline")
				}
			}
			return messages
		}

		ownershipDuration.LetValue(s, 500*time.Millisecond)
		s.Before(func(t *testcase.T) {
			for _, value := range batch.Get(t) {
				assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), value))
			}
		})

		s.Test("delivers every expected payload exactly once while all claims remain unsettled", func(t *testcase.T) {
			messages := act(t)
			var got []Entity
			for _, msg := range messages {
				assert.NoError(t, msg.Context().Err())
				got = append(got, msg.Data())
			}
			assert.ContainsExactly(t, got, batch.Get(t))
			queueV2AssertEmpty(t, ready.Get(t))
			for _, msg := range messages {
				assert.NoError(t, msg.ACK())
			}
			queueV2AssertEmpty(t, ready.Get(t))
		})
	})

	s.Describe("handler duration across database timeouts", func(s *testcase.Spec) {
		act := func(t *testcase.T) pubsub.Message[Entity] { return delivery.Get(t) }

		ownershipDuration.LetValue(s, 500*time.Millisecond)
		connection.Let(s, func(t *testcase.T) postgresql.Connection {
			primary := connection.Super(t)
			cfg := primary.DB.Config()
			cfg.ConnConfig.RuntimeParams["statement_timeout"] = "250ms"
			cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "250ms"
			return queueV2ConnectPool(t, cfg)
		})

		s.Test("a nontransactional handler outlives both server timeouts while short operations and ACK remain healthy", func(t *testcase.T) {
			msg := act(t)
			conn := connection.Get(t)
			_, transactional := conn.LookupTx(msg.Context())
			assert.False(t, transactional)
			var statementTimeout, idleTimeout string
			assert.NoError(t, conn.QueryRowContext(queueV2Context(t), `
				SELECT current_setting('statement_timeout'), current_setting('idle_in_transaction_session_timeout')
			`).Scan(&statementTimeout, &idleTimeout))
			assert.Equal(t, statementTimeout, "250ms")
			assert.Equal(t, idleTimeout, "250ms")

			// Only handler work waits; every database interaction is a short query or settlement.
			time.Sleep(750 * time.Millisecond)
			assert.NoError(t, msg.Context().Err())
			assert.NoError(t, conn.DB.Ping(queueV2Context(t)))
			queueV2AssertEmpty(t, ready.Get(t))
			assert.NoError(t, msg.ACK())
			queueV2AssertEmpty(t, ready.Get(t))
		})
	})

	s.Describe("idle subscriptions", func(s *testcase.Spec) {
		type received struct {
			value Entity
			err   error
		}
		act := func(t *testcase.T) <-chan received {
			q := ready.Get(t)
			ctx, cancel := context.WithTimeout(context.Background(), queueV2OperationBudget)
			results := make(chan received, len(batch.Get(t)))
			var workers sync.WaitGroup
			for range batch.Get(t) {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for msg, err := range q.Subscribe(ctx) {
						if err != nil {
							results <- received{err: err}
							return
						}
						results <- received{value: msg.Data(), err: msg.ACK()}
						return
					}
				}()
			}
			done := make(chan struct{})
			go func() {
				workers.Wait()
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(queueV2OperationBudget):
					t.Error("idle subscriptions did not terminate after cancellation")
				}
			})
			return results
		}

		poolSize.LetValue(s, int32(1))
		config.Let(s, func(t *testcase.T) queueV2Config {
			cfg := config.Super(t)
			cfg.EmptyQueueBreakTime = 500 * time.Millisecond
			return cfg
		})

		s.Test("more idle subscribers than connections release the pool and make progress after publishing", func(t *testcase.T) {
			q := ready.Get(t)
			pool := q.Connection.DB
			assert.Must(t).Equal(pool.Stat().MaxConns(), int32(1))
			before := pool.Stat().AcquireCount()
			results := act(t)
			assert.True(t, len(batch.Get(t)) > int(pool.Stat().MaxConns()))
			assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
				stats := pool.Stat()
				assert.True(it, stats.AcquireCount()-before >= int64(len(batch.Get(t))), "wait for idle claim polls")
				assert.Equal(it, stats.AcquiredConns(), int32(0), "polling must not reserve a connection during the wait")
			})
			assert.NoError(t, pool.Ping(queueV2Context(t)))
			for _, value := range batch.Get(t) {
				assert.NoError(t, q.Publish(queueV2Context(t), value))
			}
			var got []Entity
			ctx := queueV2Context(t)
			for range batch.Get(t) {
				select {
				case result := <-results:
					assert.NoError(t, result.err)
					got = append(got, result.value)
				case <-ctx.Done():
					t.Fatal("idle subscriptions prevented pool progress")
				}
			}
			assert.ContainsExactly(t, got, batch.Get(t))
		})
	})

	s.Describe("Message#ACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return delivery.Get(t).ACK() }

		s.Test("does not redeliver an acknowledged message after its subscription stops", func(t *testcase.T) {
			assert.NoError(t, act(t))
			deliverySub.Get(t).Stop()
			queueV2AssertEmpty(t, ready.Get(t))
		})

		s.When("the delivery was already acknowledged", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) { assert.NoError(t, delivery.Get(t).ACK()) })

			s.Then("repeating ACK cannot resurrect the message", func(t *testcase.T) {
				assert.NoError(t, act(t))
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("the delivery was released and a replacement owns the message", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, delivery.Get(t).NACK())
				replacement.Get(t)
			})

			s.Then("the former delivery cannot acknowledge or release the replacement", func(t *testcase.T) {
				assert.Equal(t, replacement.Get(t).Data(), data.Get(t))
				assert.ErrorIs(t, act(t), postgresql.ErrQueueMessageSettled)
				assert.NoError(t, replacement.Get(t).Context().Err())
				queueV2AssertEmpty(t, ready.Get(t))
				assert.NoError(t, replacement.Get(t).NACK())
				recovered := queueV2Subscribe(t, ready.Get(t)).Receive(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				assert.NoError(t, recovered.ACK())
			})
		})

		s.When("the subscription is canceled while ownership remains valid", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				delivery.Get(t)
				deliverySub.Get(t).Cancel()
			})

			s.Then("ACK still succeeds although the handler context is ordinarily canceled", func(t *testcase.T) {
				msg := delivery.Get(t)
				queueV2AwaitCanceled(t, msg.Context(), queueV2OperationBudget)
				assert.ErrorIs(t, context.Cause(msg.Context()), context.Canceled)
				assert.False(t, errors.Is(context.Cause(msg.Context()), postgresql.ErrQueueFencingLeaseLost))
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))
				deliverySub.Get(t).Stop()
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("a short ownership lifetime is configured", func(s *testcase.Spec) {
			ownershipDuration.LetValue(s, 500*time.Millisecond)

			s.Then("confirmed ACK remains permanent past the old lease lifetime and on a new queue instance", func(t *testcase.T) {
				assert.NoError(t, act(t))
				deliverySub.Get(t).Stop()
				time.Sleep(2 * ownershipDuration.Get(t))
				restarted := makeQueue.Get(t)(config.Get(t))
				queueV2AssertEmpty(t, restarted)
				assert.NoError(t, act(t))
			})
		})
		s.TODO("ownership expires during ACK: confirmed completion and a valid replacement delivery cannot both win")

	})

	s.Describe("Message#NACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return delivery.Get(t).NACK() }

		s.Test("releases the unchanged payload for another delivery", func(t *testcase.T) {
			assert.NoError(t, act(t))
			replacement := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, replacement.Data(), data.Get(t))
			assert.NoError(t, replacement.ACK())
		})

		s.When("the delivery was already acknowledged", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) { assert.NoError(t, delivery.Get(t).ACK()) })

			s.Then("NACK cannot reverse the confirmed acknowledgement", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), postgresql.ErrQueueMessageSettled)
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("a prior NACK has already allowed a replacement delivery", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, delivery.Get(t).NACK())
				replacement.Get(t)
			})

			s.Then("repeating NACK cannot release the replacement delivery", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, replacement.Get(t).Context().Err())
				queueV2AssertEmpty(t, ready.Get(t))
				assert.NoError(t, replacement.Get(t).ACK())
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

	})

	s.Describe("settlement response loss at the adapter boundary", func(s *testcase.Spec) {
		var (
			acknowledge  = let.Var(s, func(t *testcase.T) bool { return true })
			responseLost = let.Var(s, func(t *testcase.T) error {
				return errors.New("injected lost settlement response: " + t.Random.UUID())
			})
			settlementCalls   = let.Var(s, func(t *testcase.T) *atomic.Int32 { return new(atomic.Int32) })
			responseLossArmed = let.Var(s, func(t *testcase.T) *atomic.Bool { return new(atomic.Bool) })
			healthy           = let.Var(s, func(t *testcase.T) queueV2Subject {
				cfg := config.Get(t)
				cfg.Connection = connection.Get(t)
				return makeQueue.Get(t)(cfg)
			})
		)
		act := func(t *testcase.T) error {
			if acknowledge.Get(t) {
				return delivery.Get(t).ACK()
			}
			return delivery.Get(t).NACK()
		}

		ownershipDuration.LetValue(s, 500*time.Millisecond)
		config.Let(s, func(t *testcase.T) queueV2Config {
			cfg := config.Super(t)
			adapter := cfg.Connection.DBAdapter
			injected, calls, armed := responseLost.Get(t), settlementCalls.Get(t), responseLossArmed.Get(t)
			// Arm only after setup so migration-state queries keep their real responses.
			// Native claims and renewals, and the healthy observer, bypass this adapter;
			// no private SQL matching is needed.
			cfg.Connection.DBAdapter = func(pool *pgxpool.Pool) flsql.Queryable {
				q := adapter(pool)
				return flsql.QueryableAdapter{
					ExecFunc:  q.ExecContext,
					QueryFunc: q.QueryContext,
					QueryRowFunc: func(ctx context.Context, query string, args ...any) flsql.Row {
						row := q.QueryRowContext(ctx, query, args...)
						if !armed.Load() {
							return row
						}
						calls.Add(1)
						return queueV2LostResponseRow{Row: row, Err: injected}
					},
				}
			}
			return cfg
		})
		s.Before(func(t *testcase.T) {
			delivery.Get(t)
			assert.Must(t).Equal(settlementCalls.Get(t).Load(), int32(0), "setup must not consume a settlement response")
			responseLossArmed.Get(t).Store(true)
		})

		s.Test("an applied ACK remains permanent past the lease lifetime while its handle retains the same uncertainty", func(t *testcase.T) {
			err := act(t)
			assert.ErrorIs(t, err, postgresql.ErrQueueOutcomeUncertain)
			assert.ErrorIs(t, err, responseLost.Get(t))
			assert.Equal(t, settlementCalls.Get(t).Load(), int32(1))
			assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
			for range 3 {
				assert.True(t, delivery.Get(t).ACK() == err, "repeated ACK must return the same terminal error")
				assert.True(t, delivery.Get(t).NACK() == err, "opposite settlement must return the same terminal error")
			}
			deliverySub.Get(t).Stop()
			time.Sleep(2 * ownershipDuration.Get(t))
			queueV2AssertEmpty(t, healthy.Get(t))
			assert.True(t, delivery.Get(t).ACK() == err)
			assert.True(t, delivery.Get(t).NACK() == err)
			assert.Equal(t, settlementCalls.Get(t).Load(), int32(1), "terminal uncertainty must not retry database settlement")
		})

		s.When("the lost response belongs to NACK", func(s *testcase.Spec) {
			acknowledge.LetValue(s, false)

			s.Then("the applied release permits redelivery but neither repeated settlement can affect the replacement", func(t *testcase.T) {
				err := act(t)
				assert.ErrorIs(t, err, postgresql.ErrQueueOutcomeUncertain)
				assert.ErrorIs(t, err, responseLost.Get(t))
				assert.Equal(t, settlementCalls.Get(t).Load(), int32(1))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 1)
				recovered := queueV2Subscribe(t, healthy.Get(t)).Receive(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				for range 3 {
					assert.True(t, delivery.Get(t).NACK() == err, "repeated NACK must return the same terminal error")
					assert.True(t, delivery.Get(t).ACK() == err, "opposite settlement must return the same terminal error")
				}
				deliverySub.Get(t).Stop()
				assert.Equal(t, settlementCalls.Get(t).Load(), int32(1), "terminal uncertainty must not retry database settlement")
				assert.NoError(t, recovered.Context().Err())
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 1)
				queueV2AssertEmpty(t, healthy.Get(t))
				assert.NoError(t, recovered.ACK())
				queueV2AssertEmpty(t, healthy.Get(t))
				assert.True(t, delivery.Get(t).ACK() == err)
				assert.True(t, delivery.Get(t).NACK() == err)
				assert.Equal(t, settlementCalls.Get(t).Load(), int32(1))
			})
		})
	})

	s.Describe("competing ACK and NACK", func(s *testcase.Spec) {
		type result struct {
			ack bool
			err error
		}
		act := func(t *testcase.T) []result {
			msg := delivery.Get(t)
			const callers = 32
			start := make(chan struct{})
			results := make(chan result, callers)
			var waiting, workers sync.WaitGroup
			waiting.Add(callers)
			workers.Add(callers)
			for i := range callers {
				go func() {
					defer workers.Done()
					ack := i%2 == 0
					waiting.Done()
					<-start
					if ack {
						results <- result{ack: true, err: msg.ACK()}
					} else {
						results <- result{ack: false, err: msg.NACK()}
					}
				}()
			}
			done := make(chan struct{})
			go func() {
				workers.Wait()
				close(done)
			}()
			t.Cleanup(func() {
				select {
				case <-done:
				case <-time.After(queueV2OperationBudget):
					t.Error("competing settlement calls did not finish")
				}
			})
			waiting.Wait()
			close(start)
			select {
			case <-done:
			case <-time.After(queueV2OperationBudget):
				t.Fatal("competing settlement calls did not complete within the operation budget")
			}
			var got []result
			for range callers {
				got = append(got, <-results)
			}
			return got
		}

		s.Test("confirms only one kind of settlement and makes every same or opposite call agree with the durable outcome", func(t *testcase.T) {
			results := act(t)
			var ackWon, confirmed bool
			for _, got := range results {
				if got.err == nil {
					ackWon, confirmed = got.ack, true
					break
				}
			}
			assert.Must(t).True(confirmed, "one settlement must succeed")
			for _, got := range results {
				if got.ack == ackWon {
					assert.NoError(t, got.err)
				} else {
					assert.ErrorIs(t, got.err, postgresql.ErrQueueMessageSettled)
				}
			}

			if ackWon {
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
				assert.NoError(t, delivery.Get(t).ACK())
				assert.ErrorIs(t, delivery.Get(t).NACK(), postgresql.ErrQueueMessageSettled)
			} else {
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 1)
				recovered := replacement.Get(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				for range 3 {
					assert.NoError(t, delivery.Get(t).NACK())
					assert.ErrorIs(t, delivery.Get(t).ACK(), postgresql.ErrQueueMessageSettled)
				}
				assert.NoError(t, recovered.Context().Err())
				queueV2AssertEmpty(t, ready.Get(t))
				assert.NoError(t, recovered.ACK())
			}
			deliverySub.Get(t).Stop()
			queueV2AssertEmpty(t, ready.Get(t))
		})
	})

	s.Describe("delivery ownership", func(s *testcase.Spec) {
		act := func(t *testcase.T) pubsub.Message[Entity] { return delivery.Get(t) }

		ownershipDuration.LetValue(s, 500*time.Millisecond)

		s.Test("renews an untouched delivery beyond its initial lifetime without pinning a connection", func(t *testcase.T) {
			msg := act(t)
			assert.Equal[any](t, msg.Context().Value(queueV2ContextKey{}), callerValue.Get(t))
			time.Sleep(2 * ownershipDuration.Get(t))
			assert.NoError(t, msg.Context().Err())
			assert.NoError(t, connection.Get(t).DB.Ping(queueV2Context(t)))
			queueV2AssertEmpty(t, ready.Get(t))
			assert.NoError(t, msg.ACK())
		})

		s.When("all connections are held so renewal cannot reach the database", func(s *testcase.Spec) {
			poolSize.LetValue(s, int32(1))
			var release = let.Var(s, func(t *testcase.T) func() {
				delivery.Get(t)
				return queueV2StarvePool(t, connection.Get(t).DB)
			})
			s.Before(func(t *testcase.T) { release.Get(t) })

			s.Then("signals ownership loss without settlement or iterator progress", func(t *testcase.T) {
				msg := act(t)
				queueV2AwaitCanceled(t, msg.Context(), 3*ownershipDuration.Get(t))
				assert.ErrorIs(t, context.Cause(msg.Context()), postgresql.ErrQueueFencingLeaseLost)
				release.Get(t)()
				assert.ErrorIs(t, msg.ACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.ErrorIs(t, msg.NACK(), postgresql.ErrQueueFencingLeaseLost)
			})

			s.Then("recovery never revalidates stale handles or lets them settle the replacement", func(t *testcase.T) {
				stale := act(t)
				queueV2AwaitCanceled(t, stale.Context(), 3*ownershipDuration.Get(t))
				assert.ErrorIs(t, context.Cause(stale.Context()), postgresql.ErrQueueFencingLeaseLost)
				release.Get(t)()
				recovered := replacement.Get(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				for range 2 {
					assert.ErrorIs(t, stale.ACK(), postgresql.ErrQueueFencingLeaseLost)
					assert.ErrorIs(t, stale.NACK(), postgresql.ErrQueueFencingLeaseLost)
				}
				assert.ErrorIs(t, context.Cause(stale.Context()), postgresql.ErrQueueFencingLeaseLost)
				assert.NoError(t, recovered.Context().Err())
				queueV2AssertEmpty(t, ready.Get(t))
				assert.NoError(t, recovered.ACK())
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("caller cancellation stops renewal without advancing the iterator", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				delivery.Get(t)
				deliverySub.Get(t).Cancel()
			})

			s.Then("keeps the ordinary cancellation cause but rejects settlement after the lease expires", func(t *testcase.T) {
				stale := act(t)
				queueV2AwaitCanceled(t, stale.Context(), queueV2OperationBudget)
				assert.ErrorIs(t, context.Cause(stale.Context()), context.Canceled)
				time.Sleep(2 * ownershipDuration.Get(t))
				assert.ErrorIs(t, stale.ACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.ErrorIs(t, stale.NACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.ErrorIs(t, context.Cause(stale.Context()), context.Canceled)
				recovered := replacement.Get(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				assert.NoError(t, recovered.ACK())
			})
		})
	})

	s.Describe("iterator cleanup", func(s *testcase.Spec) {
		var advance = let.Var(s, func(t *testcase.T) bool { return false })
		act := func(t *testcase.T) pubsub.Message[Entity] {
			sub := deliverySub.Get(t)
			if advance.Get(t) {
				return sub.Receive(t)
			}
			sub.Stop()
			return queueV2Subscribe(t, ready.Get(t)).Receive(t)
		}

		s.Before(func(t *testcase.T) { delivery.Get(t) })

		s.Test("stopping an unsettled delivery makes it recoverable without acknowledging it", func(t *testcase.T) {
			recovered := act(t)
			assert.Equal(t, recovered.Data(), data.Get(t))
			assert.Error(t, delivery.Get(t).ACK())
			assert.NoError(t, recovered.Context().Err())
			assert.NoError(t, recovered.ACK())
		})

		s.When("the consumer advances without settling the current message", func(s *testcase.Spec) {
			advance.LetValue(s, true)

			s.Then("redelivers it instead of silently acknowledging or leaving it permanently owned", func(t *testcase.T) {
				recovered := act(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				assert.Error(t, delivery.Get(t).ACK())
				assert.NoError(t, recovered.Context().Err())
				assert.NoError(t, recovered.ACK())
			})
		})
	})

	s.Describe("decode failure", func(s *testcase.Spec) {
		var decodeError = let.Var(s, func(t *testcase.T) error { return errors.New(t.Random.String()) })
		act := func(t *testcase.T) (pubsub.Message[Entity], error, bool) {
			return queueV2Subscribe(t, ready.Get(t)).Next()
		}

		ownershipDuration.LetValue(s, 500*time.Millisecond)
		s.Before(func(t *testcase.T) {
			assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
		})

		s.Test("delivers decodable payloads", func(t *testcase.T) {
			msg, err, ok := act(t)
			assert.Must(t).True(ok)
			assert.NoError(t, err)
			assert.Must(t).NotNil(msg)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
		})

		s.When("successful decoding takes longer than the ownership lifetime", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				delay := 2 * ownershipDuration.Get(t)
				cfg.Codec = struct {
					codec.Marshaler
					codec.Unmarshaler
				}{jsonkit.Codec{}, codec.UnmarshalerFunc(func(data []byte, ptr any) error {
					time.Sleep(delay)
					return (jsonkit.Codec{}).Unmarshal(data, ptr)
				})}
				return cfg
			})

			s.Then("renews ownership during decoding and yields a healthy delivery that can be acknowledged", func(t *testcase.T) {
				started := time.Now()
				msg, err, ok := act(t)
				assert.Must(t).True(ok)
				assert.NoError(t, err)
				assert.Must(t).NotNil(msg)
				assert.True(t, time.Since(started) > ownershipDuration.Get(t))
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.Context().Err())
				queueV2AssertEmpty(t, ready.Get(t))
				assert.NoError(t, msg.ACK())
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("the configured codec cannot decode the message", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.Codec = queueV2BadDecoder{Codec: jsonkit.Codec{}, Err: decodeError.Get(t)}
				return cfg
			})

			s.Then("reports the decoding failure rather than yielding a processable delivery", func(t *testcase.T) {
				_, err, ok := act(t)
				assert.True(t, ok)
				assert.ErrorIs(t, err, decodeError.Get(t))
			})

			s.Then("a healthy decoder recovers the unchanged payload without waiting for subscription cleanup", func(t *testcase.T) {
				_, err, ok := act(t)
				assert.True(t, ok)
				assert.ErrorIs(t, err, decodeError.Get(t))
				cfg := config.Get(t)
				cfg.Codec = jsonkit.Codec{}
				healthy := makeQueue.Get(t)(cfg)
				msg := queueV2Subscribe(t, healthy).Receive(t)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.ACK())
				queueV2AssertEmpty(t, healthy)
			})
		})
	})

	s.Describe("#Migrate", func(s *testcase.Spec) {
		act := func(t *testcase.T) error { return subject.Get(t).Migrate(queueV2Context(t)) }

		s.Test("creates message-only storage supporting Publish ACK and Purge without receipts", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
			assert.ContainsExactly(t, queueV2Indexes(t, connection.Get(t), name.Get(t)), []string{
				name.Get(t) + "_pkey",
				name.Get(t) + "_order",
			})
			q := subject.Get(t)
			assert.NoError(t, q.Publish(queueV2Context(t), data.Get(t)))
			msg := queueV2Subscribe(t, q).Receive(t)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
			assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
			for _, value := range batch.Get(t) {
				assert.NoError(t, q.Publish(queueV2Context(t), value))
			}
			assert.NoError(t, q.Purge(queueV2Context(t)))
			assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
			queueV2AssertEmpty(t, q)
			assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
		})

		s.Test("creates distinct message tables without receipts for nonblocking queues", func(t *testcase.T) {
			assert.NoError(t, act(t))
			other.Get(t)
			assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{
				name.Get(t),
				name.Get(t) + "_other",
			})
		})

		s.When("the named queue already contains messages", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
			})

			s.Then("repeated migration preserves those messages", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))
				msg := queueV2Subscribe(t, subject.Get(t)).Receive(t)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.ACK())
			})
		})

		s.When("another named queue has an active delivery and a waiting message", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				q := other.Get(t)
				assert.NoError(t, q.Publish(queueV2Context(t), batch.Get(t)[0]))
				assert.NoError(t, q.Publish(queueV2Context(t), batch.Get(t)[1]))
			})

			s.Then("migration does not alter the other queue or its active delivery", func(t *testcase.T) {
				active := queueV2Subscribe(t, other.Get(t)).Receive(t)
				assert.NoError(t, act(t))
				assert.NoError(t, active.Context().Err())
				assert.NoError(t, active.ACK())
				waiting := queueV2Subscribe(t, other.Get(t)).Receive(t)
				assert.Equal(t, waiting.Data(), batch.Get(t)[1])
				assert.NoError(t, waiting.ACK())
			})
		})

		s.When("Name is missing", func(s *testcase.Spec) {
			name.LetValue(s, "")

			s.Then("returns a clear queue-name error instead of migrating fallback storage", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, strings.ToLower(err.Error()), "name")
			})
		})

		s.When("blocking publishing is enabled", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.Blocking = true
				return cfg
			})

			s.Then("creates the named receipt table and expiry index alongside message storage", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))
				assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{
					name.Get(t),
					name.Get(t) + "_receipts",
				})
				assert.ContainsExactly(t, queueV2Indexes(t, connection.Get(t), name.Get(t)), []string{
					name.Get(t) + "_pkey",
					name.Get(t) + "_order",
					name.Get(t) + "_receipts_pkey",
					name.Get(t) + "_expires_at_idx",
				})
			})
		})

		for _, validName := range []string{"a", "a0_b9", strings.Repeat("z", 40)} {
			s.When("Name is the supported boundary "+validName, func(s *testcase.Spec) {
				name.LetValue(s, validName)

				s.Then("uses the full untruncated name for the message table and its indexes", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
					assert.ContainsExactly(t, queueV2Indexes(t, connection.Get(t), name.Get(t)), []string{
						name.Get(t) + "_pkey",
						name.Get(t) + "_order",
					})
				})
			})
		}
	})

	s.Describe("#Migrator.Migrate", func(s *testcase.Spec) {
		pending := let.Var(s, func(t *testcase.T) <-chan error {
			publisher := subject.Get(t)
			publisher.Blocking = true
			m := publisher.Migrator()
			assert.Must(t).NoError(m.Migrate(queueV2Context(t)))
			result := queueV2PublishAsync(t, publisher, data.Get(t))
			assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
				assert.Equal(it, queueV2MessageCount(it, connection.Get(t), name.Get(t)), 1)
			})
			return result
		})
		act := func(t *testcase.T) error {
			m := subject.Get(t).Migrator()
			return m.Migrate(queueV2Context(t))
		}

		s.Test("repeated nonblocking migration leaves receipt storage disabled", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
		})

		s.When("blocking publishing is enabled after a nonblocking migration", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.Blocking = true
				return cfg
			})
			s.Before(func(t *testcase.T) {
				q := subject.Get(t)
				q.Blocking = false
				m := q.Migrator()
				assert.Must(t).NoError(m.Migrate(queueV2Context(t)))
				assert.Must(t).ContainsExactly(queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
				for _, value := range batch.Get(t) {
					assert.Must(t).NoError(q.Publish(queueV2Context(t), value))
				}
			})

			s.Then("adds receipt storage without losing or reordering queued messages", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))
				assert.ContainsExactly(t, queueV2Tables(t, connection.Get(t)), []string{
					name.Get(t), name.Get(t) + "_receipts",
				})
				assert.ContainsExactly(t, queueV2Indexes(t, connection.Get(t), name.Get(t)), []string{
					name.Get(t) + "_pkey", name.Get(t) + "_order",
					name.Get(t) + "_receipts_pkey", name.Get(t) + "_expires_at_idx",
				})
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), len(batch.Get(t)))
				sub := queueV2Subscribe(t, subject.Get(t))
				for _, value := range batch.Get(t) {
					msg := sub.Receive(t)
					assert.Equal(t, msg.Data(), value)
					assert.NoError(t, msg.ACK())
				}
				queueV2AssertEmpty(t, subject.Get(t))
			})
		})

		s.When("a blocking queue already has a pending publisher", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) { pending.Get(t) })

			s.Then("nonblocking migration preserves the receipt table and its pending record", func(t *testcase.T) {
				conn := connection.Get(t)
				receipts := pgx.Identifier{name.Get(t) + "_receipts"}.Sanitize()
				var id string
				var expiresAt time.Time
				assert.Must(t).NoError(conn.QueryRowContext(queueV2Context(t),
					"SELECT id, expires_at FROM "+receipts+" WHERE outcome = 'pending'",
				).Scan(&id, &expiresAt))

				assert.NoError(t, act(t))
				assert.NoError(t, act(t))
				assert.ContainsExactly(t, queueV2Tables(t, conn), []string{name.Get(t), name.Get(t) + "_receipts"})
				var outcome string
				var preservedExpiry time.Time
				assert.NoError(t, conn.QueryRowContext(queueV2Context(t),
					"SELECT outcome, expires_at FROM "+receipts+" WHERE id = $1", id,
				).Scan(&outcome, &preservedExpiry))
				assert.Equal(t, outcome, "pending")
				assert.Equal(t, preservedExpiry, expiresAt)
				queueV2AssertPublishing(t, pending.Get(t))
				msg := queueV2Subscribe(t, subject.Get(t)).Receive(t)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.ACK())
				assert.NoError(t, queueV2AwaitPublished(t, pending.Get(t)))
			})
		})
	})

	for _, operation := range []struct {
		name string
		call func(*testcase.T, queueV2Subject, Entity) error
	}{
		{"#Migrate", func(t *testcase.T, q queueV2Subject, _ Entity) error { return q.Migrate(queueV2Context(t)) }},
		{"#Publish", func(t *testcase.T, q queueV2Subject, value Entity) error { return q.Publish(queueV2Context(t), value) }},

		{"#Subscribe", func(t *testcase.T, q queueV2Subject, _ Entity) error {
			msg, err, ok := queueV2Subscribe(t, q).Next()
			assert.True(t, ok, "validation errors must be yielded")
			assert.Nil(t, msg)
			return err
		}},
		{"#Purge", func(t *testcase.T, q queueV2Subject, _ Entity) error { return q.Purge(queueV2Context(t)) }},
	} {
		s.Describe(operation.name+" name validation", func(s *testcase.Spec) {
			act := func(t *testcase.T) error { return operation.call(t, subject.Get(t), data.Get(t)) }

			for _, invalidName := range []string{
				"", "Messages", "0messages", "two-queues", "two.queues", "two queues",
				"mésages", "messages\x00", "messages\"; SELECT 1;--", strings.Repeat("a", 41),
			} {
				s.When(fmt.Sprintf("Name is %q", invalidName), func(s *testcase.Spec) {
					name.LetValue(s, invalidName)

					s.Then("returns a queue-name error without creating fallback or truncated storage", func(t *testcase.T) {
						err := act(t)
						assert.Error(t, err)
						assert.Contains(t, strings.ToLower(err.Error()), "name")
						assert.Empty(t, queueV2Tables(t, connection.Get(t)))
					})
				})
			}
		})
	}

	s.Describe("publishing from Message.Context", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return other.Get(t).Publish(delivery.Get(t).Context(), batch.Get(t)[0])
		}

		s.Before(func(t *testcase.T) {
			other.Get(t)
			delivery.Get(t)
		})

		s.Test("is nontransactional by default and NACK does not undo already published handler work", func(t *testcase.T) {
			_, transactional := connection.Get(t).LookupTx(delivery.Get(t).Context())
			assert.False(t, transactional)
			assert.Equal[any](t, delivery.Get(t).Context().Value(queueV2ContextKey{}), callerValue.Get(t))
			assert.NoError(t, act(t))
			assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)+"_other"), 1)
			assert.NoError(t, delivery.Get(t).NACK())
			published := queueV2Subscribe(t, other.Get(t)).Receive(t)
			assert.Equal(t, published.Data(), batch.Get(t)[0])
			assert.NoError(t, published.ACK())
			recovered := replacement.Get(t)
			assert.Equal(t, recovered.Data(), data.Get(t))
			assert.NoError(t, recovered.ACK())
		})

		s.When("transactional message contexts are enabled", func(s *testcase.Spec) {
			config.Let(s, func(t *testcase.T) queueV2Config {
				cfg := config.Super(t)
				cfg.TransactionalMessageContext = true
				return cfg
			})
			poolSize.LetValue(s, int32(2))
			ownershipDuration.LetValue(s, 500*time.Millisecond)

			s.Then("ACK commits handler publishing together with removal of the input message", func(t *testcase.T) {
				msg := delivery.Get(t)
				_, transactional := connection.Get(t).LookupTx(msg.Context())
				assert.True(t, transactional)
				assert.NoError(t, act(t))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)+"_other"), 0)
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 1)
				// The handler transaction pins one connection; the second must remain available for renewal.
				time.Sleep(2 * ownershipDuration.Get(t))
				assert.NoError(t, msg.Context().Err())
				assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
					assert.Equal(it, connection.Get(t).DB.Stat().AcquiredConns(), int32(1))
				})
				assert.NoError(t, msg.ACK())
				assert.NoError(t, msg.ACK())
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
				published := queueV2Subscribe(t, other.Get(t)).Receive(t)
				assert.Equal(t, published.Data(), batch.Get(t)[0])
				assert.NoError(t, published.ACK())
			})

			s.Then("NACK rolls back handler publishing before releasing the input for redelivery", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)+"_other"), 0)
				assert.NoError(t, delivery.Get(t).NACK())
				assert.NoError(t, delivery.Get(t).NACK())
				recovered := replacement.Get(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				assert.NoError(t, recovered.ACK())
				queueV2AssertEmpty(t, other.Get(t))
			})

			s.Then("ownership loss rolls back handler work and releases its connection without settlement or iteration", func(t *testcase.T) {
				msg := delivery.Get(t)
				assert.NoError(t, act(t))
				pool := connection.Get(t).DB
				// The transaction holds one connection; reserve only the spare to block renewal.
				spare, err := pool.Acquire(queueV2Context(t))
				assert.NoError(t, err)
				release := sync.OnceFunc(spare.Release)
				t.Cleanup(release)
				assert.Equal(t, pool.Stat().AcquiredConns(), int32(2))

				queueV2AwaitCanceled(t, msg.Context(), 3*ownershipDuration.Get(t))
				assert.ErrorIs(t, context.Cause(msg.Context()), postgresql.ErrQueueFencingLeaseLost)
				release()
				assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
					assert.Equal(it, pool.Stat().AcquiredConns(), int32(0))
				})
				assert.NoError(t, pool.Ping(queueV2Context(t)))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)+"_other"), 0)

				recovered := replacement.Get(t)
				assert.Equal(t, recovered.Data(), data.Get(t))
				assert.ErrorIs(t, msg.ACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.ErrorIs(t, msg.NACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.NoError(t, recovered.ACK())
				queueV2AssertEmpty(t, other.Get(t))
			})
		})
	})

	s.Describe("#Subscribe transaction options", func(s *testcase.Spec) {
		var options = let.Var(s, func(t *testcase.T) pgx.TxOptions {
			return pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite}
		})
		act := func(t *testcase.T) (pubsub.Message[Entity], error, bool) {
			ctx := postgresql.ContextTxOptions.ContextWith(callerContext.Get(t), options.Get(t))
			return queueV2SubscribeContext(t, ready.Get(t), ctx).Next()
		}

		config.Let(s, func(t *testcase.T) queueV2Config {
			cfg := config.Super(t)
			cfg.TransactionalMessageContext = true
			return cfg
		})
		s.Before(func(t *testcase.T) {
			assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
		})

		s.Test("accepts explicit read-committed read-write options for a fresh handler transaction", func(t *testcase.T) {
			msg, err, ok := act(t)
			assert.Must(t).True(ok)
			assert.NoError(t, err)
			assert.Must(t).NotNil(msg)
			_, transactional := connection.Get(t).LookupTx(msg.Context())
			assert.True(t, transactional)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
		})

		s.When("the caller explicitly requests non-deferrable transactions", func(s *testcase.Spec) {
			options.Let(s, func(t *testcase.T) pgx.TxOptions {
				opts := options.Super(t)
				opts.DeferrableMode = pgx.NotDeferrable
				return opts
			})

			s.Then("accepts the options and delivers a fresh transaction that can acknowledge", func(t *testcase.T) {
				msg, err, ok := act(t)
				assert.Must(t).True(ok)
				assert.NoError(t, err)
				assert.Must(t).NotNil(msg)
				_, transactional := connection.Get(t).LookupTx(msg.Context())
				assert.True(t, transactional)
				assert.Equal(t, msg.Data(), data.Get(t))
				assert.NoError(t, msg.ACK())
			})
		})

		for _, invalid := range []struct {
			name string
			opts pgx.TxOptions
		}{
			{"read-only", pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly}},
			{"repeatable-read", pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite}},
			{"custom-BEGIN", pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite, BeginQuery: "BEGIN"}},
			{"custom-COMMIT", pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite, CommitQuery: "COMMIT"}},
		} {
			s.When("the caller requests "+invalid.name+" transactions", func(s *testcase.Spec) {
				options.LetValue(s, invalid.opts)

				s.Then("rejects the options without retaining a connection or preventing a fresh delivery", func(t *testcase.T) {
					msg, err, ok := act(t)
					assert.True(t, ok)
					assert.Nil(t, msg)
					assert.Error(t, err)
					assert.Contains(t, strings.ToLower(err.Error()), "transaction")
					assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
						assert.Equal(it, connection.Get(t).DB.Stat().AcquiredConns(), int32(0))
					})
					recovered := queueV2Subscribe(t, ready.Get(t)).Receive(t)
					assert.Equal(t, recovered.Data(), data.Get(t))
					assert.NoError(t, recovered.ACK())
				})
			})
		}
	})

	s.Describe("#Subscribe with a transactional caller context", func(s *testcase.Spec) {
		var transaction = let.Var(s, func(t *testcase.T) context.Context {
			ready.Get(t)
			ctx, err := connection.Get(t).BeginTx(queueV2Context(t))
			assert.NoError(t, err)
			t.Cleanup(func() { _ = connection.Get(t).RollbackTx(ctx) })
			return ctx
		})
		act := func(t *testcase.T) (pubsub.Message[Entity], error, bool) {
			return queueV2SubscribeContext(t, ready.Get(t), transaction.Get(t)).Next()
		}

		s.Test("rejects an inherited transaction instead of claiming from a stale transaction snapshot", func(t *testcase.T) {
			assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
			msg, err, ok := act(t)
			assert.True(t, ok)
			assert.Nil(t, msg)
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "transaction")
			assert.NoError(t, connection.Get(t).RollbackTx(transaction.Get(t)))
			recovered := queueV2Subscribe(t, ready.Get(t)).Receive(t)
			assert.Equal(t, recovered.Data(), data.Get(t))
			assert.NoError(t, recovered.ACK())
		})
	})

	s.Describe("named queue identity", func(s *testcase.Spec) {
		act := func(t *testcase.T) queueV2Subject { return makeQueue.Get(t)(config.Get(t)) }

		s.Before(func(t *testcase.T) {
			assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
		})

		s.Test("another instance with the same name and namespace addresses the same messages", func(t *testcase.T) {
			msg := queueV2Subscribe(t, act(t)).Receive(t)
			assert.Equal(t, msg.Data(), data.Get(t))
			assert.NoError(t, msg.ACK())
			queueV2AssertEmpty(t, ready.Get(t))
		})

		s.Test("publishing, subscribing and acknowledging another name leaves this queue untouched", func(t *testcase.T) {
			assert.NoError(t, other.Get(t).Publish(queueV2Context(t), batch.Get(t)[0]))
			msg := queueV2Subscribe(t, other.Get(t)).Receive(t)
			assert.Equal(t, msg.Data(), batch.Get(t)[0])
			assert.NoError(t, msg.ACK())
			original := queueV2Subscribe(t, act(t)).Receive(t)
			assert.Equal(t, original.Data(), data.Get(t))
			assert.NoError(t, original.ACK())
		})

	})

	s.Describe("#Purge", func(s *testcase.Spec) {
		activationResult := let.Var(s, func(t *testcase.T) <-chan error { return nil })
		act := func(t *testcase.T) error { return ready.Get(t).Purge(queueV2Context(t)) }

		s.Before(func(t *testcase.T) {
			assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), data.Get(t)))
		})

		s.Test("removes waiting messages from the named queue", func(t *testcase.T) {
			assert.NoError(t, act(t))
			queueV2AssertEmpty(t, ready.Get(t))
		})

		s.When("blocking publishing is enabled immediately before eligible messages are deleted", func(s *testcase.Spec) {
			ready.Let(s, func(t *testcase.T) postgresql.Queue[Entity] {
				q := ready.Super(t)
				publisher := q
				publisher.Blocking = true
				// Activation must not compete with Purge for its transaction's connection.
				publisher.Connection = queueV2ConnectPool(t, q.Connection.DB.Config())
				activate := sync.OnceFunc(func() {
					assert.Must(t).ContainsExactly(queueV2Tables(t, connection.Get(t)), []string{name.Get(t)})
					m := publisher.Migrator()
					assert.Must(t).NoError(m.Migrate(queueV2Context(t)))
					result := queueV2PublishAsync(t, publisher, batch.Get(t)[0])
					activationResult.Set(t, result)
					assert.Eventually(t, queueV2OperationBudget, func(it testing.TB) {
						assert.Equal(it, queueV2MessageCount(it, connection.Get(t), name.Get(t)), 2)
					})
					queueV2AssertPublishing(t, result)
				})
				messageDelete := "DELETE FROM " + pgx.Identifier{q.Table}.Sanitize()
				before := func(query string) {
					if strings.Contains(query, messageDelete) {
						activate()
					}
				}
				dbAdapter, txAdapter := q.Connection.DBAdapter, q.Connection.TxAdapter
				q.Connection.DBAdapter = func(pool *pgxpool.Pool) flsql.Queryable {
					return queueV2BeforeQuery(dbAdapter(pool), before)
				}
				q.Connection.TxAdapter = func(tx *pgx.Tx) flsql.Queryable {
					return queueV2BeforeQuery(txAdapter(tx), before)
				}
				return q
			})

			s.Then("reports the purged outcome to the newly enabled blocking publisher", func(t *testcase.T) {
				assert.NoError(t, act(t))
				result := activationResult.Get(t)
				assert.Must(t).NotNil(result, "the adapter must activate blocking publishing before the delete")
				assert.ErrorIs(t, queueV2AwaitPublished(t, result), postgresql.ErrQueueMessagePurged)
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("another named queue has an active delivery and a waiting message", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, other.Get(t).Publish(queueV2Context(t), batch.Get(t)[0]))
				assert.NoError(t, other.Get(t).Publish(queueV2Context(t), batch.Get(t)[1]))
			})

			s.Then("leaves the other queue's active and waiting messages unchanged", func(t *testcase.T) {
				active := queueV2Subscribe(t, other.Get(t)).Receive(t)
				assert.NoError(t, act(t))
				assert.NoError(t, active.Context().Err())
				assert.NoError(t, active.ACK())
				waiting := queueV2Subscribe(t, other.Get(t)).Receive(t)
				assert.Equal(t, waiting.Data(), batch.Get(t)[1])
				assert.NoError(t, waiting.ACK())
			})
		})

		s.When("this queue has an active lease as well as a waiting message", func(s *testcase.Spec) {
			var active = let.Var(s, func(t *testcase.T) pubsub.Message[Entity] {
				return queueV2Subscribe(t, ready.Get(t)).Receive(t)
			})
			s.Before(func(t *testcase.T) {
				active.Get(t)
				assert.NoError(t, ready.Get(t).Publish(queueV2Context(t), batch.Get(t)[0]))
			})

			s.Then("removes only the waiting message and leaves the active owner able to acknowledge", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 1)
				assert.NoError(t, active.Get(t).Context().Err())
				assert.NoError(t, active.Get(t).ACK())
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})

		s.When("a canceled subscriber leaves an expired lease", func(s *testcase.Spec) {
			ownershipDuration.LetValue(s, 500*time.Millisecond)
			var stale = let.Var(s, func(t *testcase.T) pubsub.Message[Entity] {
				sub := queueV2Subscribe(t, ready.Get(t))
				msg := sub.Receive(t)
				sub.Cancel()
				return msg
			})
			s.Before(func(t *testcase.T) {
				stale.Get(t)
				time.Sleep(2 * ownershipDuration.Get(t))
			})

			s.Then("removes the expired message without allowing the stale handle to settle it", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, queueV2MessageCount(t, connection.Get(t), name.Get(t)), 0)
				assert.ErrorIs(t, stale.Get(t).ACK(), postgresql.ErrQueueFencingLeaseLost)
				assert.ErrorIs(t, stale.Get(t).NACK(), postgresql.ErrQueueFencingLeaseLost)
				queueV2AssertEmpty(t, ready.Get(t))
			})
		})
	})

	// These scenarios still require process/network fault injection or agreed performance thresholds.
	s.TODO("brief connectivity failure: ownership may remain valid only while assured; reconnection never revalidates a delivery that already lost ownership")
	s.TODO("sustained connectivity failure: ownership is not assumed indefinitely, Context signals loss, and recovery works when the database returns")
	s.TODO("crash recovery: kill a child consumer after delivery without running cleanup; another process recovers within the documented bound without the old consumer reconnecting")
	s.TODO("wire-protocol settlement response loss, including transactional COMMIT: complement adapter-boundary injection with real transport failure and verify terminal uncertainty")
	s.TODO("competing ACK and NACK during ownership loss while settlement is in flight: deterministically interleave expiry, database completion and replacement ownership")
	s.TODO("performance acceptance: compare Queue and QueueV2 acknowledged throughput, delivery latency and connection usage under agreed sustained, slow-handler and over-pool workloads; agree numerical regression thresholds first")
}

type queueV2Config struct {
	Name                        string
	Connection                  postgresql.Connection
	Codec                       codec.Codec
	ToMeta                      func(Entity) map[string]any
	LIFO                        bool
	SortBy                      string
	Blocking                    bool
	EmptyQueueBreakTime         time.Duration
	OwnershipDuration           time.Duration
	TransactionalMessageContext bool
}

type queueV2Subject interface {
	pubsub.Publisher[Entity]
	pubsub.Subscriber[Entity]
	migration.Migratable

	Purge(context.Context) error
}

// Test deadlock guards, NOT product ownership-loss or recovery guarantees.
const queueV2OperationBudget = 5 * time.Second
const queueV2ObservationWindow = 100 * time.Millisecond

func queueV2Context(tb testing.TB) context.Context {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), queueV2OperationBudget)
	tb.Cleanup(cancel)
	return ctx
}

func queueV2Connection(t *testcase.T, size int32) postgresql.Connection {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(DatabaseDSN(t))
	assert.NoError(t, err)
	cfg.MinConns = 0
	cfg.MaxConns = size
	schema := "queue_v2_" + strings.ReplaceAll(t.Random.UUID(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	admin := GetConnection(t)
	_, err = admin.ExecContext(queueV2Context(t), "CREATE SCHEMA "+quoted)
	assert.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queueV2OperationBudget)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		assert.NoError(t, err)
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted
	return queueV2ConnectPool(t, cfg)
}

func queueV2ConnectPool(t *testcase.T, cfg *pgxpool.Config) postgresql.Connection {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(queueV2Context(t), cfg)
	assert.NoError(t, err)
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() {
			pool.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(queueV2OperationBudget):
			t.Error("queue connection pool did not close; a connection may still be reserved")
		}
	})
	return postgresql.MakeConnectionFromPGXPool(pool)
}

type queueV2Subscription struct {
	Next   func() (pubsub.Message[Entity], error, bool)
	Stop   func()
	Cancel context.CancelFunc
}

type queueV2ContextKey struct{}

func queueV2Subscribe(tb testing.TB, q queueV2Subject) *queueV2Subscription {
	tb.Helper()
	return queueV2SubscribeContext(tb, q, context.Background())
}

func queueV2SubscribeContext(tb testing.TB, q queueV2Subject, parent context.Context) *queueV2Subscription {
	tb.Helper()
	ctx, cancel := context.WithTimeout(parent, queueV2OperationBudget)
	next, stop := iter.Pull2(q.Subscribe(ctx))
	tb.Cleanup(func() {
		cancel()
		stop()
	})
	return &queueV2Subscription{Next: next, Stop: stop, Cancel: cancel}
}

func (sub *queueV2Subscription) Receive(tb testing.TB) pubsub.Message[Entity] {
	tb.Helper()
	msg, err, ok := sub.Next()
	assert.Must(tb).NoError(err)
	assert.Must(tb).True(ok, "expected a delivery before the test operation deadline")
	assert.Must(tb).NotNil(msg)
	return msg
}

// Polls a real subscription rather than counting rows: owned rows are allowed
// to remain in storage. This observation window is not a recovery-bound test.
func queueV2AssertEmpty(tb testing.TB, q queueV2Subject) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), queueV2ObservationWindow)
	defer cancel()
	for _, err := range q.Subscribe(ctx) {
		assert.Must(tb).Error(err, "there must be no successful delivery")
		assert.ErrorIs(tb, err, context.DeadlineExceeded)
		assert.ErrorIs(tb, ctx.Err(), context.DeadlineExceeded,
			"the subscription must observe the entire window, not terminate early")
		return
	}
	assert.ErrorIs(tb, ctx.Err(), context.DeadlineExceeded,
		"an empty subscription must wait for cancellation, not terminate early")
}

func queueV2PublishAsync(tb testing.TB, q queueV2Subject, value Entity) <-chan error {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), queueV2OperationBudget)
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result <- q.Publish(ctx, value)
	}()
	tb.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(queueV2OperationBudget):
			tb.Error("blocking publisher did not stop after cancellation")
		}
	})
	return result
}

func queueV2AssertPublishing(tb testing.TB, result <-chan error) {
	tb.Helper()
	select {
	case err := <-result:
		tb.Fatalf("publisher completed before successful processing: %v", err)
	case <-time.After(queueV2ObservationWindow):
	}
}

func queueV2AwaitPublished(tb testing.TB, result <-chan error) error {
	tb.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(queueV2OperationBudget):
		tb.Fatal("publisher did not complete after settlement")
		return nil
	}
}

func queueV2Tables(tb testing.TB, conn postgresql.Connection) []string {
	tb.Helper()
	var names []string
	err := conn.QueryRowContext(queueV2Context(tb), `
		SELECT COALESCE(array_agg(c.relname::text ORDER BY c.relname), ARRAY[]::text[])
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p')
		  AND c.relname <> 'frameless_schema_migrations'
	`).Scan(&names)
	assert.Must(tb).NoError(err)
	return names
}

func queueV2Indexes(tb testing.TB, conn postgresql.Connection, name string) []string {
	tb.Helper()
	var indexes []string
	err := conn.QueryRowContext(queueV2Context(tb), `
		SELECT COALESCE(array_agg(indexname::text), ARRAY[]::text[])
		FROM pg_catalog.pg_indexes
		WHERE schemaname = current_schema() AND tablename IN ($1, $2)
	`, name, name+"_receipts").Scan(&indexes)
	assert.Must(tb).NoError(err)
	return indexes
}

func queueV2MessageCount(tb testing.TB, conn postgresql.Connection, name string) int {
	tb.Helper()
	var count int
	table := pgx.Identifier{name}.Sanitize()
	err := conn.QueryRowContext(queueV2Context(tb), "SELECT count(*) FROM "+table).Scan(&count)
	assert.Must(tb).NoError(err)
	return count
}

func queueV2AwaitCanceled(tb testing.TB, ctx context.Context, within time.Duration) {
	tb.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(within):
		tb.Fatal("delivery context did not signal cancellation within the observation bound")
	}
}

// Only hold connections from the test's dedicated pool; never fault the shared admin pool.
func queueV2StarvePool(tb testing.TB, pool *pgxpool.Pool) func() {
	tb.Helper()
	var held []*pgxpool.Conn
	release := sync.OnceFunc(func() {
		for _, conn := range held {
			conn.Release()
		}
	})
	tb.Cleanup(release)
	for range pool.Config().MaxConns {
		conn, err := pool.Acquire(queueV2Context(tb))
		assert.Must(tb).NoError(err)
		held = append(held, conn)
	}
	return release
}

func queueV2BeforeQuery(q flsql.Queryable, before func(string)) flsql.Queryable {
	return flsql.QueryableAdapter{
		ExecFunc: func(ctx context.Context, query string, args ...any) (flsql.Result, error) {
			before(query)
			return q.ExecContext(ctx, query, args...)
		},
		QueryFunc: func(ctx context.Context, query string, args ...any) (flsql.Rows, error) {
			before(query)
			return q.QueryContext(ctx, query, args...)
		},
		QueryRowFunc: func(ctx context.Context, query string, args ...any) flsql.Row {
			before(query)
			return q.QueryRowContext(ctx, query, args...)
		},
	}
}

// This is adapter-boundary fault injection, not a PostgreSQL wire-protocol harness:
// the real Scan completes the database operation before its successful response is hidden.
type queueV2LostResponseRow struct {
	flsql.Row
	Err error
}

func (r queueV2LostResponseRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	return r.Err
}

type queueV2GobCodec struct{}

func (queueV2GobCodec) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(v)
	return buf.Bytes(), err
}

func (queueV2GobCodec) Unmarshal(data []byte, ptr any) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(ptr)
}

type queueV2BadDecoder struct {
	codec.Codec
	Err error
}

func (c queueV2BadDecoder) Unmarshal([]byte, any) error { return c.Err }
