package postgresql_test

import (
	"context"
	"fmt"
	"iter"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

var _ migration.Migratable = postgresql.Queue[Entity]{}

func ExampleQueue() {

	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer cm.Close()

	q := postgresql.Queue[Entity]{
		Table:      "queue_name",
		Connection: cm,
	}

	ctx := context.Background()
	ent := Entity{Foo: "foo"}

	err = q.Publish(ctx, ent)
	if err != nil {
		panic(err)
	}

	for msg, err := range q.Subscribe(ctx) {
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
		postgresql.Queue[Entity]{Table: queueName, Connection: c}.
			Migrate(MakeContext(t)))

	basicQueue := postgresql.Queue[Entity]{
		Table:      queueName,
		Connection: c,
	}

	lifoQueue := postgresql.Queue[Entity]{
		Table:      queueName,
		Connection: c,

		LIFO: true,
	}

	blockingQueue := postgresql.Queue[Entity]{
		Table:      queueName,
		Connection: c,

		Blocking: true,
	}
	assert.NoError(t, blockingQueue.Migrate(MakeContext(t)))

	testcase.RunSuite(t,
		pubsubcontract.FIFO[Entity](basicQueue, basicQueue),
		pubsubcontract.LIFO[Entity](lifoQueue, lifoQueue),
		pubsubcontract.Durable[Entity](basicQueue, basicQueue),
		pubsubcontract.Blocking[Entity](blockingQueue, blockingQueue),
		pubsubcontract.Queue[Entity](basicQueue, basicQueue),
	)
}

func TestQueue_emptyBreakTime(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	const queueName = "test_queue_empty_queue_break_time"
	ctx := context.Background()
	now := time.Now().UTC()
	timecop.Travel(t, now)

	q := postgresql.Queue[testent.Foo]{
		Table:          queueName,
		Connection:     GetConnection(t),
		EmptyBreakTime: time.Hour,
	}
	assert.NoError(t, q.Migrate(MakeContext(t)))

	res := pubsubtest.Subscribe[testent.Foo](t, q, ctx)

	t.Log("we wait until the subscription is idle")
	time.Sleep(time.Second)

	var waitTime = time.Second
	timecop.Travel(t, waitTime)

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
	s := testcase.NewSpec(t)
	rnd := random.New(random.CryptoSeed{})
	cm := GetConnection(t)
	s.Test("single", func(t *testcase.T) {
		q1 := postgresql.Queue[testent.Foo]{
			Table:      "q42",
			Connection: cm,
		}

		assert.NoError(t, q1.Migrate(t.Context()))

		res1 := pubsubtest.Subscribe[testent.Foo](t, q1, context.Background())

		var (
			ent1A     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1B     = rnd.Make(testent.Foo{}).(testent.Foo)
			ent1C     = rnd.Make(testent.Foo{}).(testent.Foo)
			expected1 = []testent.Foo{ent1A, ent1B, ent1C}
		)

		assert.NoError(t, q1.Publish(t.Context(), ent1A))
		assert.NoError(t, q1.Publish(t.Context(), ent1B))
		assert.NoError(t, q1.Publish(t.Context(), ent1C))

		res1.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainsExactly(tb, expected1, foos)
		})
	})
	s.Test("multi", func(t *testcase.T) {
		cm := GetConnection(t)

		q1 := postgresql.Queue[testent.Foo]{
			Table:      "q42",
			Connection: cm,
		}

		assert.NoError(t, q1.Migrate(t.Context()))

		q2 := postgresql.Queue[testent.Foo]{
			Table:      "q24",
			Connection: cm,
		}

		assert.NoError(t, q2.Migrate(t.Context()))

		res1 := pubsubtest.Subscribe[testent.Foo](t, q1, t.Context())
		res2 := pubsubtest.Subscribe[testent.Foo](t, q2, t.Context())

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

		assert.NoError(t, q1.Publish(t.Context(), ent1A))
		assert.NoError(t, q1.Publish(t.Context(), ent1B))
		assert.NoError(t, q1.Publish(t.Context(), ent1C))

		assert.NoError(t, q2.Publish(t.Context(), ent2A))
		assert.NoError(t, q2.Publish(t.Context(), ent2B))
		assert.NoError(t, q2.Publish(t.Context(), ent2C))

		t.Cleanup(func() {
			if !t.Failed() {
				return
			}
			t.Log("res1", pp.Format(res1.Values()))
			t.Log("res2", pp.Format(res2.Values()))
		})

		res1.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainsExactly(tb, expected1, foos)
		})

		res2.Eventually(t, func(tb testing.TB, foos []testent.Foo) {
			assert.ContainsExactly(tb, expected2, foos)
		})
	})
}

func BenchmarkQueue(b *testing.B) {
	const queueName = "test_entity"
	var (
		ctx = MakeContext(b)
		rnd = random.New(random.CryptoSeed{})
		cm  = GetConnection(b)
		q   = postgresql.Queue[Entity]{
			Table:      queueName,
			Connection: cm,
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
		vs := random.Slice(b.N, func() Entity {
			return Entity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		})

		for _, v := range vs {
			assert.NoError(b, q.Publish(ctx, v))
		}

		sub := q.Subscribe(ctx)
		assert.NotNil(b, sub)

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
			for _, msg := range msgs {
				_ = q.Publish(ctx, msg)
			}
		}
	})
}

func BenchmarkQueue_mvp(b *testing.B) {
	values := random.Slice(b.N, func() testent.Foo {
		return testent.MakeFoo(b)
	})

	var q = postgresql.Queue[testent.Foo]{
		Table:      "queue_benchmark_foo",
		Connection: GetConnection(b),
	}

	b.Run("#Publish", func(b *testing.B) {
		assert.NoError(b, q.Purge(b.Context()))
		assert.Equal(b, len(values), b.N)
		b.ResetTimer()

		for _, v := range values {
			_ = q.Publish(b.Context(), v)
		}
	})

	b.Run("#Subscribe", func(b *testing.B) {
		assert.NoError(b, q.Purge(b.Context()))

		for _, v := range values {
			_ = q.Publish(b.Context(), v)
		}

		sub := q.Subscribe(b.Context())

		for range 42 {
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}

		b.ResetTimer()

		var n = b.N
		for msg, err := range sub {
			if err != nil {
				assert.NoError(b, err)
			}
			_ = msg.ACK()
			n--
			if n == 0 {
				return
			}
		}
	})

}
