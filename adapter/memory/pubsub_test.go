package memory_test

import (
	"context"
	"iter"
	"sort"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/testing/testent"
)

var _ interface {
	pubsub.Publisher[testent.Foo]
	pubsub.Subscriber[testent.Foo]
} = &memory.Queue[testent.Foo]{}

func TestQueue_implementsFIFO(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.FIFO[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsLIFO(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{LIFO: true}

	testcase.RunSuite(t, pubsubcontract.LIFO[TestEntity](q, q, pubsubConfig))
}

func TestQueue_implementsBuffered(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.Durable[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsVolatile(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{Volatile: true}

	testcase.RunSuite(t, pubsubcontract.Volatile[TestEntity](q, q, pubsubConfig))
}

func TestQueue_implementsBlocking(t *testing.T) {
	q := &memory.Queue[TestEntity]{Blocking: true}

	pubsubcontract.Blocking(q, q, pubsubcontract.Config[TestEntity]{
		// SupportPublishContextCancellation: true,// TODO: fixme in memory queue

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}).Test(t)
}

func TestQueue_implementsOrdering(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
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

	pubsubcontract.Ordering(q, q, sorting, pubsubConfig).Test(t)
}

var _ pubsub.Publisher[testent.Foo] = &memory.FanOutExchange[testent.Foo]{}

func TestQueue_implementsFanOutExchange(t *testing.T) {
	exchange := &memory.FanOutExchange[testent.Foo]{}

	var MakeQueue = func(tb testing.TB) pubsub.Subscriber[testent.Foo] {
		return exchange.MakeQueue()
	}

	testcase.RunSuite(t,
		pubsubcontract.Broadcast[testent.Foo](exchange, MakeQueue),
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}

func TestQueue_implementsTransactionalMessageContext(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		MakeContext: func(t testing.TB) context.Context {
			return context.Background()
		},
		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.TransactionalMessageContext(q, q, pubsubConfig).Test(t)
}

func TestQueue_implementsTransactionalPublisher(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		MakeContext: func(t testing.TB) context.Context {
			return context.Background()
		},
		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.TransactionalPublisher(q, q, q, pubsubConfig).Test(t)
}

var _ pubsub.Publisher[testent.Foo] = &memory.FanOutExchange[testent.Foo]{}

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
		pubsubcontract.Queue[testent.Foo](q, q),
		pubsubcontract.Durable[testent.Foo](q, q),
		pubsubcontract.FIFO[testent.Foo](q, q),
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
	assert.NoError(t, q.Publish(ctx, ent1))
	assert.NoError(t, q.Publish(ctx, ent2))
	// t.Log(pp.Format(q))

	t.Log("#1 subscribe to queue")
	sub1 := q.Subscribe(ctx)
	assert.NotNil(t, sub1)

	sub1Next, sub1Stop := iter.Pull2(sub1)
	defer sub1Stop()

	msg1, err, ok := sub1Next()
	assert.NoError(t, err)
	t.Log("fetching the first message in #1 sub")
	assert.True(t, ok)

	t.Log("ent1 should have been received")
	assert.Equal(t, ent1, msg1.Data())
	t.Log("intentionally not ACKing the message, to prove subscriptions don't step on each other's foot")

	t.Log("#2 subscribe to queue")
	sub2 := q.Subscribe(ctx)
	assert.NotNil(t, sub2)

	sub2Next, sub2Stop := iter.Pull2(sub2)
	defer sub2Stop()

	t.Log("#2 sub next")
	msg2, err, ok := sub2Next()
	assert.NoError(t, err)
	assert.True(t, ok)
	t.Log("ent2 should be received")
	assert.Equal(t, ent2, msg2.Data())

	t.Log("then sub1 ack the message")
	assert.NoError(t, msg1.ACK())

	t.Log("then sub1 next will hang since no more message present in the queue")
	w := assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
		msg1, err, hasNext := sub1Next()
		assert.NoError(t, err)
		t.Log("then eventually next will return back with a new value")
		assert.True(t, hasNext)
		t.Log("and this new value is the ent2 that was NACKed")
		assert.Equal(t, ent2, msg1.Data())
		assert.NoError(t, msg1.ACK())
	})

	t.Log("when ent2 is NACKed")
	assert.NoError(t, msg2.NACK())

	w.Wait() // wait till NotWithin assertion finish its thing
}

func TestQueue_blockingQueueIsNotCompatibleWithTransactions(t *testing.T) {
	t.Run("BeginTx fails", func(t *testing.T) {
		q := &memory.Queue[testent.Foo]{
			Blocking: true,
		}

		_, err := q.BeginTx(t.Context())
		assert.Error(t, err)
	})
	t.Run("Publish with tx fails", func(t *testing.T) {
		q := &memory.Queue[testent.Foo]{
			Blocking: false,
		}

		tx, err := q.BeginTx(t.Context())
		assert.NoError(t, err)

		q.Blocking = true

		assert.Error(t, q.Publish(tx, testent.MakeFoo(t)))
	})
}

// TestFanOutExchange_MakeQueue_concurrent covers that queues created at the very
// same moment all end up bound to the exchange.
//
// A FanOutExchange keeps the queues bound to it in a plain slice that MakeQueue
// grows. Callers commonly make their queue on the goroutine that is about to
// consume it, so MakeQueue calls overlap easily, and two overlapping appends can
// read the same slice and then write over each other's result.
//
// The queue that loses is still handed back to its caller looking perfectly
// healthy, while the exchange never learns that it exists. The symptom is a
// consumer that stays silent forever, which is indistinguishable from simply
// having no traffic, so the mistake is expensive to track down in a live system.
func TestFanOutExchange_MakeQueue_concurrent(t *testing.T) {
	const numQueue = 32

	exchange := &memory.FanOutExchange[testent.Foo]{}

	var (
		queues = make([]*memory.Queue[testent.Foo], numQueue)
		start  = make(chan struct{})
		wg     sync.WaitGroup
	)
	for i := range queues {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // line the goroutines up, so the calls truly overlap
			queues[i] = exchange.MakeQueue()
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, exchange.Len(), numQueue,
		"expected that every queue made by the exchange is also bound to it")

	// The bookkeeping is only a means to an end: what a queue is made for is to
	// receive what the exchange broadcasts.
	var expected = testent.MakeFoo(t)
	assert.NoError(t, exchange.Publish(t.Context(), expected))

	for i, q := range queues {
		assert.Within(t, time.Second, func(ctx context.Context) {
			next, stop := iter.Pull2(q.Subscribe(ctx))
			defer stop()

			msg, err, ok := next()
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, msg.Data(), expected)
			assert.NoError(t, msg.ACK())
		}, assert.MessageF("queue #%d was not served by the exchange's broadcast", i))
	}
}

var _ pubsub.Subscriber[testent.Foo] = &memory.FanOutExchange[testent.Foo]{}

// TestFanOutExchange_pubsubSubscriber covers the exchange in its pubsub.Subscriber role.
//
// Subscribing to the exchange directly binds a queue that lives only as long as
// the subscription, which makes the exchange a volatile pubsub: a broadcast that
// happens while nobody is subscribed is gone for good. FanOutExchange.MakeQueue
// remains the durable alternative, covered by TestQueue_implementsFanOutExchange.
func TestFanOutExchange_pubsubSubscriber(t *testing.T) {
	exchange := &memory.FanOutExchange[testent.Foo]{}

	pubsubcontract.Volatile[testent.Foo](exchange, exchange, pubsubcontract.Config[testent.Foo]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) testent.Foo {
			return testent.MakeFoo(tb)
		},
	}).Test(t)
}

// TestFanOutExchange_Subscribe_broadcast covers that subscribing to the exchange
// fans out, rather than the subscriptions competing over the messages the way
// subscriptions of a single Queue do.
func TestFanOutExchange_Subscribe_broadcast(t *testing.T) {
	exchange := &memory.FanOutExchange[testent.Foo]{}

	var MakeSubscriber = func(tb testing.TB) pubsub.Subscriber[testent.Foo] {
		return makeBoundSubscriber(tb, exchange)
	}

	pubsubcontract.Broadcast[testent.Foo](exchange, MakeSubscriber).Test(t)
}

// makeBoundSubscriber gives pubsubcontract.Broadcast what it asks for from
// MakeSubscriber: a subscriber that is already bound to the exchange by the time
// it is returned, so that it also receives what is published before anything
// starts to consume it.
//
// Subscribing is the very act that binds a queue to the exchange, so the
// subscription is made here and then handed out ready-made.
func makeBoundSubscriber[Data any](tb testing.TB, exchange *memory.FanOutExchange[Data]) *boundSubscriber[Data] {
	ctx, unbind := context.WithCancel(context.Background())
	tb.Cleanup(unbind)
	return &boundSubscriber[Data]{
		exchange: exchange,
		sub:      exchange.Subscribe(ctx),
		unbind:   unbind,
	}
}

type boundSubscriber[Data any] struct {
	exchange *memory.FanOutExchange[Data]
	sub      pubsub.Subscription[Data]
	unbind   func()
}

func (s *boundSubscriber[Data]) Subscribe(ctx context.Context) pubsub.Subscription[Data] {
	// The subscription already has a context of its own, so the consumer's
	// context is bridged onto it. Without this, cancelling the consumer would
	// leave the subscription running, and whoever waits for it to finish hangs.
	stop := context.AfterFunc(ctx, s.unbind)
	return func(yield func(pubsub.Message[Data], error) bool) {
		defer stop()
		for msg, err := range s.sub {
			if !yield(msg, err) {
				return
			}
		}
	}
}

// Purge lets the contract's clean-ahead flush the exchange, instead of falling
// back on draining the subscription, which would end it before the test that
// asked for the subscriber gets to use it.
func (s *boundSubscriber[Data]) Purge(ctx context.Context) error {
	return s.exchange.Purge(ctx)
}

// TestFanOutExchange_Subscribe_queueLifetime covers when a subscription's queue
// is bound to the exchange, and when it is released again.
//
// Binding late would lose whatever is published between Subscribe and the moment
// the caller starts to consume the subscription, which commonly happens on
// another goroutine. Never unbinding would leave the exchange broadcasting into
// a queue that grows with no one left to drain it.
func TestFanOutExchange_Subscribe_queueLifetime(t *testing.T) {
	t.Run("the queue is bound before the subscription is consumed", func(t *testing.T) {
		exchange := &memory.FanOutExchange[testent.Foo]{}

		sub := exchange.Subscribe(t.Context())
		assert.Equal(t, exchange.Len(), 1,
			"expected that Subscribe binds its queue right away")

		// this is published in the window that a lazily bound queue would lose
		expected := testent.MakeFoo(t)
		assert.NoError(t, exchange.Publish(t.Context(), expected))

		assert.Within(t, time.Second, func(context.Context) {
			next, stop := iter.Pull2(sub)
			defer stop()

			msg, err, ok := next()
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, msg.Data(), expected,
				"expected to receive what was published right after Subscribe returned")
			assert.NoError(t, msg.ACK())
		})
	})

	t.Run("the queue is unbound when the subscription is done being consumed", func(t *testing.T) {
		exchange := &memory.FanOutExchange[testent.Foo]{}

		sub := exchange.Subscribe(t.Context())
		assert.NoError(t, exchange.Publish(t.Context(), testent.MakeFoo(t)))

		assert.Within(t, time.Second, func(context.Context) {
			for msg, err := range sub {
				assert.NoError(t, err)
				assert.NoError(t, msg.ACK())
				break // done consuming
			}
		})

		assert.Equal(t, exchange.Len(), 0,
			"expected that the subscription's queue is unbound from the exchange")
	})

	t.Run("the queue is unbound when a subscription is left unconsumed", func(t *testing.T) {
		exchange := &memory.FanOutExchange[testent.Foo]{}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		_ = exchange.Subscribe(ctx) // never ranged over
		assert.Equal(t, exchange.Len(), 1)

		t.Log("when the subscription's context is done, the queue has no way to be consumed anymore")
		cancel()

		assert.Eventually(t, time.Second, func(t testing.TB) {
			assert.Equal(t, exchange.Len(), 0,
				"expected that the subscription's queue is unbound from the exchange")
		})
	})
}
