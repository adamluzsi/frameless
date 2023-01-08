package pubsubcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/frameless/spechelper/frcasserts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"sync"
	"testing"
	"time"
)

type PubSub[V any] interface {
	pubsub.Publisher[V]
	pubsub.Subscriber[V]
	crud.Purger
}

type pubsubBase[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeValue   func(testing.TB) V
}

func (c pubsubBase[V]) Spec(s *testcase.Spec) {
	s.Context(fmt.Sprintf("%s behaves like ", c.getPubSubTypeName()), func(s *testcase.Spec) {
		sub := c.GivenWeHaveSubscription(s)
		c.WhenIsEmpty(s)

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

			s.Before(func(t *testcase.T) {
				t.Must.Nil(c.subject().Get(t).Purge(c.MakeContext(t)))
				t.Defer(c.subject().Get(t).Purge, c.MakeContext(t))
			})

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
			c.WhenIsEmpty(s)

			s.Then("subscription didn't received anything", func(t *testcase.T) {
				frcasserts.Waiter.Wait()
				t.Must.Empty(sub.Get(t).Values())
			})
		})

		s.When("an event is published", func(s *testcase.Spec) {
			val := testcase.Let(s, spechelper.ToLet(c.MakeValue))

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
	sih := subscriptionIteratorHelper[V]{Subscriber: c.subject().Get(t)}
	sih.Start(t, c.MakeContext(t))
	t.Defer(sih.Close)
	return &sih
}

type subscriptionIteratorHelper[V any] struct {
	Subscriber       pubsub.Subscriber[V]
	HandlingDuration time.Duration

	mutex  sync.Mutex
	cancel func()
	data   []V

	ReceivedAt time.Time
}

func (sih *subscriptionIteratorHelper[V]) Values() []V {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return append([]V{}, sih.data...)
}

func (sih *subscriptionIteratorHelper[V]) LastMessageReceivedAt() time.Time {
	sih.mutex.Lock()
	defer sih.mutex.Unlock()
	return sih.ReceivedAt
}

func (sih *subscriptionIteratorHelper[V]) Start(tb testing.TB, ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	tb.Cleanup(cancel)
	sih.cancel = cancel
	go sih.wrk(tb, ctx)
}

func (sih *subscriptionIteratorHelper[V]) wrk(tb testing.TB, ctx context.Context) {
	iter := sih.Subscriber.Subscribe(ctx)
	for iter.Next() {
		sih.mutex.Lock()
		msg := iter.Value()
		sih.ReceivedAt = time.Now().UTC()
		time.Sleep(sih.HandlingDuration)
		sih.data = append(sih.data, msg.Data())
		assert.Should(tb).NoError(msg.ACK())
		sih.mutex.Unlock()
	}
	assert.Should(tb).NoError(iter.Err())
}

func (sih *subscriptionIteratorHelper[V]) Close() error {
	if sih.cancel != nil {
		sih.cancel()
	}
	return nil
}

func (c pubsubBase[V]) GivenWeHaveSubscription(s *testcase.Spec) testcase.Var[*subscriptionIteratorHelper[V]] {
	return testcase.Let(s, func(t *testcase.T) *subscriptionIteratorHelper[V] {
		return c.newSubscriptionIteratorHelper(t)
	}).EagerLoading(s)
}

func (c pubsubBase[V]) GivenWeHadSubscriptionBefore(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("given the subscription was at least once made")
		sub := c.newSubscriptionIteratorHelper(t)
		t.Must.NoError(sub.Close())
	})
}

func (c pubsubBase[V]) MakeSubscription(t *testcase.T) iterators.Iterator[pubsub.Message[V]] {
	ctx, cancel := context.WithCancel(c.MakeContext(t))
	testcase.Append(t, c.cancelFnsVar(), cancel)
	t.Defer(cancel)
	return c.subject().Get(t).Subscribe(ctx)
}

func (c pubsubBase[V]) WhenIsEmpty(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("when the publisher is empty")
		t.Must.Nil(c.subject().Get(t).Purge(c.MakeContext(t)))
		frcasserts.Waiter.Wait()
	})
}

func (c pubsubBase[V]) WhenWePublish(s *testcase.Spec, vars ...testcase.Var[V]) {
	s.Before(func(t *testcase.T) {
		var vals []V
		for _, v := range vars {
			vals = append(vals, v.Get(t))
		}
		t.Must.NoError(c.subject().Get(t).Publish(c.MakeContext(t), vals...))
		frcasserts.Waiter.Wait()
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

type stubSubscriber[V any] struct {
	HandleFunc func(ctx context.Context, msg V) error

	m      sync.Mutex
	values []V
}

func (qs *stubSubscriber[V]) Messages() []V {
	qs.m.Lock()
	defer qs.m.Unlock()
	return qs.values
}

func (qs *stubSubscriber[V]) Handle(ctx context.Context, v V) error {
	qs.m.Lock()
	defer qs.m.Lock()
	if qs.HandleFunc != nil {
		return qs.HandleFunc(ctx, v)
	}
	qs.values = append(qs.values, v)
	return nil // TODO: cover Handle -> error case
}

func (qs *stubSubscriber[V]) HandleError(ctx context.Context, err error) error {
	panic(err)
}
