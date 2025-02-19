package memory_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontracts"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/spechelper/testent"
	. "go.llib.dev/frameless/spechelper/testent"
)

var _ interface {
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
} = &memory.Queue[Foo]{}

func TestQueue_implementsFIFO(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontracts.FIFO[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsLIFO(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{LIFO: true}

	testcase.RunSuite(t, pubsubcontracts.LIFO[TestEntity](q, q, pubsubConfig))
}

func TestQueue_implementsBuffered(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontracts.Buffered[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsVolatile(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{Volatile: true}

	testcase.RunSuite(t, pubsubcontracts.Volatile[TestEntity](q, q, pubsubConfig))
}

func TestQueue_implementsBlocking(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		// SupportPublishContextCancellation: true,// TODO: fixme in memory queue

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{Blocking: true}

	pubsubcontracts.Blocking[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsOrdering(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		SortLessFunc: func(i TestEntity, j TestEntity) bool {
			return i.Data < j.Data
		},
	}

	sorting := func(entities []TestEntity) {
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].Data < entities[j].Data
		})
	}

	pubsubcontracts.Ordering[TestEntity](q, q, sorting, pubsubConfig).Test(t)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

func TestQueue_implementsFanOutExchange(t *testing.T) {
	exchange := &memory.FanOutExchange[Foo]{}

	var MakeQueue = func(tb testing.TB) pubsub.Subscriber[Foo] {
		return exchange.MakeQueue()
	}

	testcase.RunSuite(t,
		pubsubcontracts.FanOut[Foo](exchange, MakeQueue),
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

// TestQueue_combined
//
// @flaky
func TestQueue_combined(t *testing.T) {
	q := &memory.Queue[testent.Foo]{
		LIFO:     false,
		Volatile: false,
		Blocking: false,
	}

	testcase.RunSuite(t,
		pubsubcontracts.Queue[testent.Foo](q, q),
		pubsubcontracts.Buffered[testent.Foo](q, q),
		pubsubcontracts.FIFO[testent.Foo](q, q),
	)
}

func TestQueue_smoke(t *testing.T) {
	t.Log("create a FIFO Queue")
	q := &memory.Queue[testent.Foo]{}

	ctx := context.Background()
	ent1 := testent.Foo{
		ID:  "1",
		Foo: "bar",
	}
	ent2 := testent.Foo{
		ID:  "2",
		Foo: "baz",
	}

	t.Log("publish entities (ent1, ent2)")
	assert.NoError(t, q.Publish(ctx, ent1, ent2))
	// t.Log(pp.Format(q))

	t.Log("#1 subscribe to queue")
	sub1, err := q.Subscribe(ctx)
	t.Log("sub created without an error")
	assert.NoError(t, err)
	t.Log("sub by default returns back a zero message when .Value() is accessed")
	assert.NotNil(t, sub1.Value(), "zero message value is expected")
	t.Log("fetching the first message in #1 sub")
	assert.True(t, sub1.Next())

	t.Log("ent1 should have been received")
	assert.Equal(t, ent1, sub1.Value().Data())
	t.Log("intentionally not ACKing the message, to prove subscriptions don't step on each other's foot")

	t.Log("#2 subscribe to queue")
	sub2, err := q.Subscribe(ctx)
	assert.NoError(t, err)
	t.Log("#2 sub next")
	assert.True(t, sub2.Next())
	t.Log("ent2 should be received")
	assert.Equal(t, ent2, sub2.Value().Data())

	t.Log("then sub1 ack the message")
	assert.NoError(t, sub1.Value().ACK())

	t.Log("then sub1 next will hang since no more message present in the queue")
	w := assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
		hasNext := sub1.Next()
		t.Log("then eventually next will return back with a new value")
		assert.True(t, hasNext)
		t.Log("and this new value is the ent2 that was NACKed")
		assert.Equal(t, ent2, sub1.Value().Data())
		assert.NoError(t, sub1.Value().ACK())
	})

	t.Log("when ent2 is NACKed")
	assert.NoError(t, sub2.Value().NACK())

	w.Wait() // wait till NotWithin assertion finish its thing
}
