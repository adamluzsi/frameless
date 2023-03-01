package pubsubcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/spechelper"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/testcase/let"

	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type PubSub[V any] interface {
	pubsub.Publisher[V]
	pubsub.Subscriber[V]
}

type pubsubBase[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeValue   func(testing.TB) V
}

func (c pubsubBase[V]) Spec(s *testcase.Spec) {
	s.Context(fmt.Sprintf("%s behaves like ", c.getPubSubTypeName()), func(s *testcase.Spec) {
		c.TryCleanup(s)
		sub := c.GivenWeHaveSubscription(s)

		s.Before(func(t *testcase.T) {
			pubsubtest.Waiter.Wait()
		})

		s.Describe(".Publish", func(s *testcase.Spec) {
			var (
				ctx = testcase.Let(s, func(t *testcase.T) context.Context {
					return c.MakeContext(t)
				})
				msgs = testcase.Let[[]V](s, func(t *testcase.T) []V {
					var vs []V
					for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
						vs = append(vs, c.MakeValue(t))
					}
					return vs
				})
			)
			act := func(t *testcase.T) error {
				return c.subject().Get(t).Publish(ctx.Get(t), msgs.Get(t)...)
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
			val := let.With[V](s, c.MakeValue)

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

func (c pubsubBase[V]) getPubSubTypeName() string {
	return reflects.SymbolicName(*new(V))
}

func (c pubsubBase[V]) subject() testcase.Var[PubSub[V]] {
	return testcase.Var[PubSub[V]]{
		ID:   fmt.Sprintf("PubSub<%T>", *new(V)),
		Init: func(t *testcase.T) PubSub[V] { return c.MakeSubject(t) },
	}
}

func (c pubsubBase[V]) cancelFnsVar() testcase.Var[[]func()] {
	return testcase.Var[[]func()]{
		ID: "Subscription Context Cancel Fn",
		Init: func(t *testcase.T) []func() {
			return []func(){}
		},
	}
}

func (c pubsubBase[V]) newSubscriptionIteratorHelper(t *testcase.T) *subscriptionIteratorHelper[V] {
	return &subscriptionIteratorHelper[V]{Subscriber: c.subject().Get(t)}
}

type subscriptionIteratorHelper[V any] struct {
	Subscriber       pubsub.Subscriber[V]
	HandlingDuration time.Duration

	mutex sync.Mutex
	data  []V
	wg    sync.WaitGroup

	receivedAt time.Time
	ackedAt    time.Time
	cancel     func()
}

func (sih *subscriptionIteratorHelper[V]) AckedAt() time.Time {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return sih.ackedAt
}

func (sih *subscriptionIteratorHelper[V]) Values() []V {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return append([]V{}, sih.data...)
}

func (sih *subscriptionIteratorHelper[V]) ReceivedAt() time.Time {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return sih.receivedAt
}

func (sih *subscriptionIteratorHelper[V]) Start(tb testing.TB, ctx context.Context) {
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

func (sih *subscriptionIteratorHelper[V]) Stop() {
	if sih.cancel != nil {
		sih.cancel()
	}
}

func (sih *subscriptionIteratorHelper[V]) wrk(tb testing.TB, ctx context.Context, wg *sync.WaitGroup) {
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

func (c pubsubBase[V]) GivenWeHaveSubscription(s *testcase.Spec) testcase.Var[*subscriptionIteratorHelper[V]] {
	return testcase.Let(s, func(t *testcase.T) *subscriptionIteratorHelper[V] {
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.MakeContext(t))
		t.Cleanup(sih.Stop)
		return sih
	}).EagerLoading(s)
}

func (c pubsubBase[V]) GivenWeHadSubscriptionBefore(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("given the subscription was at least once made")
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.MakeContext(t))
		sih.Stop()
	})
}

func (c pubsubBase[V]) MakeSubscription(t *testcase.T) iterators.Iterator[pubsub.Message[V]] {
	ctx, cancel := context.WithCancel(c.MakeContext(t))
	testcase.Append(t, c.cancelFnsVar(), cancel)
	t.Defer(cancel)
	return c.subject().Get(t).Subscribe(ctx)
}

func (c pubsubBase[V]) TryCleanup(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), c.subject().Get(t))
		pubsubtest.Waiter.Wait()
	})
}

func (c pubsubBase[V]) WhenWePublish(s *testcase.Spec, vars ...testcase.Var[V]) {
	s.Before(func(t *testcase.T) {
		//var vals []V
		for _, v := range vars {
			t.Must.NoError(c.subject().Get(t).Publish(c.MakeContext(t), v.Get(t)))
			pubsubtest.Waiter.Wait()
			//vals = append(vals, v.Get(t))
		}
		//t.Must.NoError(c.subject().Get(t).Publish(c.MakeContext(t), vals...))

	})
}

func (c pubsubBase[V]) EventuallyIt(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[V]]], blk func(it assert.It, actual []V)) {
	var (
		actual []V
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

func (c pubsubBase[V]) EventuallyEqual(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[V]]], expected []V) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []V) {
		it.Must.Equal(expected, actual)
	})
}

func (c pubsubBase[V]) EventuallyContainExactly(t *testcase.T, subscription testcase.Var[iterators.Iterator[pubsub.Message[V]]], expected []V) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []V) {
		it.Must.ContainExactly(expected, actual)
	})
}
