package postgresql_test

import (
	"context"
	"fmt"
	"iter"
	"os"
	"reflect"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub/pubsubcontracts"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

var _ migration.Migratable = postgresql.Queue[Entity, EntityDTO]{}

func ExampleQueue() {

	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer cm.Close()

	q := postgresql.Queue[Entity, EntityDTO]{
		Name:       "queue_name",
		Connection: cm,
		Mapping:    EntityJSONMapping{},
	}

	ctx := context.Background()
	ent := Entity{Foo: "foo"}

	err = q.Publish(ctx, ent)
	if err != nil {
		panic(err)
	}

	sub, err := q.Subscribe(ctx)
	if err != nil {
		panic(err)
	}

	for msg, err := range sub {
		if err != nil {
			break
		}
		fmt.Println(msg.Data())
		_ = msg.ACK()
	}
}

func TestQueue(t *testing.T) {
	const queueName = "test_entity"
	c := GetConnection(t)

	assert.NoError(t,
		postgresql.Queue[Entity, EntityDTO]{Name: queueName, Connection: c}.
			Migrate(MakeContext(t)))

	mapping := EntityJSONMapping{}

	basicQueue := postgresql.Queue[Entity, EntityDTO]{
		Name:       queueName,
		Connection: c,
		Mapping:    mapping,
	}

	lifoQueue := postgresql.Queue[Entity, EntityDTO]{
		Name:       queueName,
		Connection: c,
		Mapping:    mapping,

		LIFO: true,
	}

	blockingQueue := postgresql.Queue[Entity, EntityDTO]{
		Name:       queueName,
		Connection: c,
		Mapping:    mapping,

		Blocking: true,
	}

	testcase.RunSuite(t,
		pubsubcontracts.FIFO[Entity](basicQueue, basicQueue),
		pubsubcontracts.LIFO[Entity](lifoQueue, lifoQueue),
		pubsubcontracts.Buffered[Entity](basicQueue, basicQueue),
		pubsubcontracts.Blocking[Entity](blockingQueue, blockingQueue),
		pubsubcontracts.Queue[Entity](basicQueue, basicQueue),
	)
}

func TestQueue_emptyQueueBreakTime(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	const queueName = "TestQueue_emptyQueueBreakTime"
	ctx := context.Background()
	now := time.Now().UTC()
	timecop.Travel(t, now)

	q := postgresql.Queue[testent.Foo, testent.FooDTO]{
		Name:                queueName,
		Connection:          GetConnection(t),
		Mapping:             testent.FooJSONMapping(),
		EmptyQueueBreakTime: time.Hour,
	}
	assert.NoError(t, q.Migrate(MakeContext(t)))

	res := pubsubtest.Subscribe[testent.Foo](t, q, ctx)

	t.Log("we wait until the subscription is idle")
	time.Sleep(time.Second)
	// idler, ok := res.Subscription().(interface{ IsIdle() bool })
	// assert.True(t, ok)
	// assert.Eventually(t, 5*time.Second, func(it assert.It) {
	// 	it.Should.True(idler.IsIdle())
	// })

	waitTime := 256 * time.Millisecond
	time.Sleep(waitTime)

	foo := testent.MakeFoo(t)
	assert.NoError(t, q.Publish(ctx, foo))

	assert.NotWithin(t, waitTime, func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				for _, got := range res.Values() {
					if reflect.DeepEqual(foo, got) {
						return
					}
				}
			}
		}
	})

	timecop.Travel(t, time.Hour+time.Second)

	assert.Within(t, waitTime, func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				for _, got := range res.Values() {
					if reflect.DeepEqual(foo, got) {
						return
					}
				}
			}
		}
	})
}

func TestQueue_smoke(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	cm := GetConnection(t)
	t.Run("single", func(t *testing.T) {
		q1 := postgresql.Queue[testent.Foo, testent.FooDTO]{
			Name:       "42",
			Connection: cm,
			Mapping:    testent.FooJSONMapping(),
		}

		res1 := pubsubtest.Subscribe[testent.Foo](t, q1, context.Background())

		var (
			ent1A     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1B     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1C     = rnd.Make(testent.Foo{}).(testent.Foo)
			expected1 = []testent.Foo{ent1A, ent1B, ent1C}
		)

		assert.NoError(t, q1.Publish(context.Background(), ent1A, ent1B, ent1C))

		res1.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainExactly(tb, expected1, foos)
		})
	})
	t.Run("multi", func(t *testing.T) {
		cm := GetConnection(t)

		q1 := postgresql.Queue[testent.Foo, testent.FooDTO]{
			Name:       "42",
			Connection: cm,
			Mapping:    testent.FooJSONMapping(),
		}

		q2 := postgresql.Queue[testent.Foo, testent.FooDTO]{
			Name:       "24",
			Connection: cm,
			Mapping:    testent.FooJSONMapping(),
		}

		res1 := pubsubtest.Subscribe[testent.Foo](t, q1, context.Background())
		res2 := pubsubtest.Subscribe[testent.Foo](t, q2, context.Background())

		var (
			rnd = random.New(random.CryptoSeed{})

			ent1A     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1B     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1C     = rnd.Make(testent.Foo{}).(testent.Foo)
			expected1 = []testent.Foo{ent1A, ent1B, ent1C}

			ent2A     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent2B     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent2C     = rnd.Make(testent.Foo{}).(testent.Foo)
			expected2 = []testent.Foo{ent2A, ent2B, ent2C}
		)

		assert.NoError(t, q1.Publish(context.Background(), ent1A, ent1B, ent1C))
		assert.NoError(t, q2.Publish(context.Background(), ent2A, ent2B, ent2C))

		t.Cleanup(func() {
			if !t.Failed() {
				return
			}
			t.Log("res1", pp.Format(res1.Values()))
			t.Log("res2", pp.Format(res2.Values()))
		})

		res1.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainExactly(tb, expected1, foos)
		})

		res2.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainExactly(tb, expected2, foos)
		})
	})
}

func BenchmarkQueue(b *testing.B) {
	const queueName = "test_entity"
	var (
		ctx = MakeContext(b)
		rnd = random.New(random.CryptoSeed{})
		cm  = GetConnection(b)
		q   = postgresql.Queue[Entity, EntityDTO]{
			Name:       queueName,
			Connection: cm,
			Mapping:    EntityJSONMapping{},
		}
	)

	b.Run("single publish", func(b *testing.B) {
		assert.NoError(b, q.Purge(ctx))
		msgs := random.Slice(b.N, func() Entity {
			return Entity{
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
		assert.NoError(b, q.Publish(ctx, random.Slice(b.N, func() Entity {
			return Entity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		})...))

		sub, err := q.Subscribe(ctx)
		assert.NoError(b, err)

		next, stop := iter.Pull2(sub)
		defer stop()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err, ok := next()
			assert.True(b, ok)
			assert.NoError(b, err)
		}
	})

	b.Run("batch publish 100", func(b *testing.B) {
		assert.NoError(b, q.Purge(ctx))
		msgs := random.Slice(100, func() Entity {
			return Entity{
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
