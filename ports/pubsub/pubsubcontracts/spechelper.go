package pubsubcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/spechelper"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type PubSub[Data any] struct {
	pubsub.Publisher[Data]
	pubsub.Subscriber[Data]
}

type base[Data any] func(testing.TB) baseSubject[Data]

type baseSubject[Data any] struct {
	PubSub      PubSub[Data]
	MakeContext func() context.Context
	MakeData    func() Data
}

func (c base[Data]) subject() testcase.Var[baseSubject[Data]] {
	return testcase.Var[baseSubject[Data]]{
		ID:   "baseSubject[Data]",
		Init: func(t *testcase.T) baseSubject[Data] { return c(t) },
	}
}

func (c base[Data]) Spec(s *testcase.Spec) {
	s.Context("implements the pubsub port", func(s *testcase.Spec) {
		c.TryCleanup(s)

		sub := c.GivenWeHaveSubscription(s)

		s.Before(func(t *testcase.T) {
			pubsubtest.Waiter.Wait()
		})

		s.Describe(".Publish", func(s *testcase.Spec) {
			var (
				ctx = testcase.Let(s, func(t *testcase.T) context.Context {
					return c.subject().Get(t).MakeContext()
				})
				data = testcase.Let[[]Data](s, func(t *testcase.T) []Data {
					var vs []Data
					for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
						vs = append(vs, c.subject().Get(t).MakeData())
					}
					return vs
				})
			)
			act := func(t *testcase.T) error {
				return c.subject().Get(t).PubSub.Publish(ctx.Get(t), data.Get(t)...)
			}

			s.Then("it publish without an error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})

			s.When("context has an error", func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				})

				s.Then("it returns the error of the context", func(t *testcase.T) {
					t.Must.ErrorIs(ctx.Get(t).Err(), act(t))
				})
			})
		})

		s.When("no events has been published published", func(s *testcase.Spec) {
			c.TryCleanup(s)

			s.Then("subscription didn't received anything", func(t *testcase.T) {
				pubsubtest.Waiter.Wait()
				t.Must.Empty(sub.Get(t).Values())
			})
		})

		s.When("an event is published", func(s *testcase.Spec) {
			val := testcase.Let(s, func(t *testcase.T) Data {
				return c.subject().Get(t).MakeData()
			})

			c.WhenWePublish(s, val)

			s.Then("event received through the subscription", func(t *testcase.T) {
				t.Eventually(func(it assert.It) {
					it.Must.Contain(sub.Get(t).Values(), val.Get(t))
				})
			})
		})

		// TODO add check here for MetaAccessor
	})
}

func (c base[Data]) cancelFnsVar() testcase.Var[[]func()] {
	return testcase.Var[[]func()]{
		ID: "Subscription Context Cancel Fn",
		Init: func(t *testcase.T) []func() {
			return []func(){}
		},
	}
}

func (c base[Data]) newSubscriptionIteratorHelper(t *testcase.T) *subscriptionIteratorHelper[Data] {
	return &subscriptionIteratorHelper[Data]{Subscriber: c.subject().Get(t).PubSub}
}

type subscriptionIteratorHelper[Data any] struct {
	Subscriber       pubsub.Subscriber[Data]
	HandlingDuration time.Duration

	mutex sync.Mutex
	data  []Data
	wg    sync.WaitGroup

	receivedAt time.Time
	ackedAt    time.Time
	cancel     func()
}

func (sih *subscriptionIteratorHelper[Data]) AckedAt() time.Time {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return sih.ackedAt
}

func (sih *subscriptionIteratorHelper[Data]) Values() []Data {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return append([]Data{}, sih.data...)
}

func (sih *subscriptionIteratorHelper[Data]) ReceivedAt() time.Time {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return sih.receivedAt
}

func (sih *subscriptionIteratorHelper[Data]) Start(tb testing.TB, ctx context.Context) {
	assert.Nil(tb, sih.cancel)
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go sih.wrk(tb, ctx, &wg)
	sih.cancel = func() {
		cancel()
		wg.Wait()
		sih.cancel = nil
	}
}

func (sih *subscriptionIteratorHelper[Data]) Stop() {
	if sih.cancel != nil {
		sih.cancel()
	}
}

func (sih *subscriptionIteratorHelper[Data]) wrk(tb testing.TB, ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	iter := sih.Subscriber.Subscribe(ctx)
	for iter.Next() {
		sih.mutex.Lock()
		msg := iter.Value()
		assert.Should(tb).NotNil(msg)
		if msg == nil {
			break
		}
		sih.receivedAt = time.Now().UTC()
		time.Sleep(sih.HandlingDuration)
		sih.data = append(sih.data, msg.Data())
		sih.ackedAt = time.Now().UTC()
		pubsubtest.Waiter.Wait()
		assert.Should(tb).NoError(msg.ACK())
		sih.mutex.Unlock()
	}
	assert.Should(tb).AnyOf(func(a *assert.AnyOf) {
		// TODO: survey which behaviour is more natural
		a.Test(func(t assert.It) { t.Must.ErrorIs(ctx.Err(), iter.Err()) })
		a.Test(func(t assert.It) { t.Must.NoError(iter.Err()) })
	})
}

func (c base[Data]) GivenWeHaveSubscription(s *testcase.Spec) testcase.Var[*subscriptionIteratorHelper[Data]] {
	return testcase.Let(s, func(t *testcase.T) *subscriptionIteratorHelper[Data] {
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.subject().Get(t).MakeContext())
		t.Cleanup(sih.Stop)
		return sih
	}).EagerLoading(s)
}

func (c base[Data]) GivenWeHadSubscriptionBefore(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("given the subscription was at least once made")
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.subject().Get(t).MakeContext())
		sih.Stop()
	})
}

func (c base[Data]) MakeSubscription(t *testcase.T) iterators.Iterator[pubsub.Message[Data]] {
	ctx, cancel := context.WithCancel(c.subject().Get(t).MakeContext())
	testcase.Append(t, c.cancelFnsVar(), cancel)
	t.Defer(cancel)
	return c.subject().Get(t).PubSub.Subscribe(ctx)
}

func (c base[Data]) TryCleanup(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		if !spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(), c.subject().Get(t).PubSub.Subscriber) {
			c.drainQueue(t, c.subject().Get(t).PubSub)
		}
		pubsubtest.Waiter.Wait()
	})
}

var DrainTimeout = 256 * time.Millisecond

func (c base[Data]) drainQueue(t *testcase.T, sub pubsub.Subscriber[Data]) {
	res := pubsubtest.Subscribe[Data](t, sub, c.subject().Get(t).MakeContext())
	defer res.Finish()
	refTime := time.Now().UTC()
	pubsubtest.Waiter.While(func() bool {
		receivedAt := res.ReceivedAt()
		if receivedAt.IsZero() {
			receivedAt = refTime
		}
		return time.Now().UTC().Sub(receivedAt) <= DrainTimeout
	})
}

func (c base[Data]) WhenWePublish(s *testcase.Spec, vars ...testcase.Var[Data]) {
	s.Before(func(t *testcase.T) {
		for _, v := range vars {
			// we publish one by one intentionally to make the tests more deterministic.
			t.Must.NoError(c.subject().Get(t).PubSub.Publish(c.subject().Get(t).MakeContext(), v.Get(t)))
			pubsubtest.Waiter.Wait()
		}
	})
}

func (c base[Data]) EventuallyIt(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[Data]]], blk func(it assert.It, actual []Data)) {
	var (
		actual []Data
		lock   sync.Mutex
	)
	go func() {
		i := subscription.Get(t)
		for i.Next() {
			lock.Lock()
			m := i.Value()
			actual = append(actual, m.Data())
			t.Must.NoError(m.ACK())
			lock.Unlock()
		}
		t.Must.NoError(i.Err())
		t.Must.NoError(i.Close())
	}()
	t.Eventually(func(t assert.It) {
		blk(t, actual)
	})
}

func (c base[Data]) EventuallyEqual(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[Data]]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []Data) {
		it.Must.Equal(expected, actual)
	})
}

func (c base[Data]) EventuallyContainExactly(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[Data]]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []Data) {
		it.Must.ContainExactly(expected, actual)
	})
}
