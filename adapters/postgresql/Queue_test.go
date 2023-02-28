package postgresql_test

import (
	"github.com/adamluzsi/frameless/adapters/postgresql"
	sh "github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/ports/migration"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

var _ migration.Migratable = postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{}

func TestQueue(t *testing.T) {
	const queueName = "test_entity"
	cm := NewConnectionManager(t)

	assert.NoError(t,
		postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{Name: queueName, ConnectionManager: cm}.
			Migrate(sh.MakeContext(t)))

	mapping := sh.TestEntityJSONMapping{}

	testcase.RunSuite(t,
		pubsubcontracts.FIFO[sh.TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[sh.TestEntity] {
				return postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
					Name:              queueName,
					ConnectionManager: cm,
					Mapping:           mapping,
				}
			},
			MakeContext: sh.MakeContext,
			MakeV:       sh.MakeTestEntity,
		},
		pubsubcontracts.LIFO[sh.TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[sh.TestEntity] {
				return postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
					Name:              queueName,
					ConnectionManager: cm,
					Mapping:           mapping,

					LIFO: true,
				}
			},
			MakeContext: sh.MakeContext,
			MakeV:       sh.MakeTestEntity,
		},
		pubsubcontracts.Buffered[sh.TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[sh.TestEntity] {
				return postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
					Name:              queueName,
					ConnectionManager: cm,
					Mapping:           mapping,
				}
			},
			MakeContext: sh.MakeContext,
			MakeV:       sh.MakeTestEntity,
		},
		pubsubcontracts.Blocking[sh.TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[sh.TestEntity] {
				return postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
					Name:              queueName,
					ConnectionManager: cm,
					Mapping:           mapping,

					Blocking: true,
				}
			},
			MakeContext: sh.MakeContext,
			MakeV:       sh.MakeTestEntity,
		},
		pubsubcontracts.Queue[sh.TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[sh.TestEntity] {
				return postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
					Name:              queueName,
					ConnectionManager: cm,
					Mapping:           mapping,
				}
			},
			MakeContext: sh.MakeContext,
			MakeV:       sh.MakeTestEntity,
		},
	)
}

func BenchmarkQueue(b *testing.B) {
	const queueName = "test_entity"
	var (
		ctx = sh.MakeContext(b)
		rnd = random.New(random.CryptoSeed{})
		cm  = NewConnectionManager(b)
		q   = postgresql.Queue[sh.TestEntity, sh.TestEntityDTO]{
			Name:              queueName,
			ConnectionManager: cm,
			Mapping:           sh.TestEntityJSONMapping{},
		}
	)

	b.Run("single publish", func(b *testing.B) {
		assert.NoError(b, q.Purge(ctx))
		msgs := random.Slice(b.N, func() sh.TestEntity {
			return sh.TestEntity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = q.Publish(ctx, msgs[i])
		}
	})

	b.Run("single element fetch", func(b *testing.B) {
		assert.NoError(b, q.Purge(ctx))
		assert.NoError(b, q.Publish(ctx, random.Slice(b.N, func() sh.TestEntity {
			return sh.TestEntity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		})...))
		sub := q.Subscribe(ctx)
		defer sub.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !sub.Next() {
				b.FailNow()
			}
			_ = sub.Value()
		}
	})

	b.Run("batch publish 100", func(b *testing.B) {
		assert.NoError(b, q.Purge(ctx))
		msgs := random.Slice(100, func() sh.TestEntity {
			return sh.TestEntity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = q.Publish(ctx, msgs...)
		}
	})
}
