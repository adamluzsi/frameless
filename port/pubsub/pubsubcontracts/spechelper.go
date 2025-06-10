package pubsubcontracts

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

type PubSub[Data any] struct {
	pubsub.Publisher[Data]
	pubsub.Subscriber[Data]
}

type base[Data any] func(testing.TB) baseSubject[Data]

type baseSubject[Data any] struct {
	Publisher   pubsub.Publisher[Data]
	Subscriber  pubsub.Subscriber[Data]
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data
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
					return c.subject().Get(t).MakeContext(t)
				})
				data = testcase.Let[[]Data](s, func(t *testcase.T) []Data {
					var vs []Data
					for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
						vs = append(vs, c.subject().Get(t).MakeData(t))
					}
					return vs
				})
			)
			act := func(t *testcase.T) error {
				return c.subject().Get(t).Publisher.Publish(ctx.Get(t), data.Get(t)...)
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
				return c.subject().Get(t).MakeData(t)
			})

			c.WhenWePublish(s, val)

			s.Then("event received through the subscription", func(t *testcase.T) {
				t.Eventually(func(it *testcase.T) {
					it.Must.Contain(sub.Get(t).Values(), val.Get(t))
				})
			})
		})

		// TODO add check here for MetaAccessor
	})
}

func (c base[Data]) newSubscriptionIteratorHelper(t *testcase.T) *subscriptionIteratorHelper[Data] {
	return &subscriptionIteratorHelper[Data]{id: t.Random.UUID(), Subscriber: c.subject().Get(t).Subscriber}
}

type subscriptionIteratorHelper[Data any] struct {
	id string

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
	sub := sih.Subscriber.Subscribe(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go sih.wrk(tb, ctx, &wg, sub)
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

func (sih *subscriptionIteratorHelper[Data]) wrk(tb testing.TB, ctx context.Context, wg *sync.WaitGroup, sub pubsub.Subscription[Data]) {
	defer wg.Done()
	for msg, err := range sub {
		if err != nil {
			assert.Should(tb).AnyOf(func(a *assert.A) {
				// TODO: survey which behaviour is more natural
				a.Test(func(t assert.It) { t.Must.ErrorIs(ctx.Err(), err) })
				a.Test(func(t assert.It) { t.Must.NoError(err) })
			})
			continue
		}
		sih.mutex.Lock()
		assert.Should(tb).True(msg != nil, "msg should not be nil")
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
}

func (c base[Data]) GivenWeHaveSubscription(s *testcase.Spec) testcase.Var[*subscriptionIteratorHelper[Data]] {
	return testcase.Let(s, func(t *testcase.T) *subscriptionIteratorHelper[Data] {
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.subject().Get(t).MakeContext(t))
		t.Cleanup(sih.Stop)
		return sih
	}).EagerLoading(s)
}

func (c base[Data]) GivenWeHadSubscriptionBefore(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("given the subscription was at least once made")
		sih := c.newSubscriptionIteratorHelper(t)
		sih.Start(t, c.subject().Get(t).MakeContext(t))
		sih.Stop()
	})
}

func (c base[Data]) MakeSubscription(t *testcase.T) pubsub.Subscription[Data] {
	ctx, cancel := context.WithCancel(c.subject().Get(t).MakeContext(t))
	t.Defer(cancel)
	return c.subject().Get(t).Subscriber.Subscribe(ctx)
}

func (c base[Data]) TryCleanup(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		if !spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(t), c.subject().Get(t).Subscriber) {
			c.drainQueue(t, c.subject().Get(t).Subscriber)
		}
		pubsubtest.Waiter.Wait()
	})
}

var DrainTimeout = 256 * time.Millisecond

func (c base[Data]) drainQueue(t *testcase.T, sub pubsub.Subscriber[Data]) {
	res := pubsubtest.Subscribe[Data](t, sub, c.subject().Get(t).MakeContext(t))
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
			t.Must.NoError(c.subject().Get(t).Publisher.Publish(c.subject().Get(t).MakeContext(t), v.Get(t)))
			pubsubtest.Waiter.Wait()
		}
	})
}

func (c base[Data]) EventuallyIt(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], blk func(it assert.It, actual []Data)) {
	var (
		actual []Data
		lock   sync.Mutex
	)
	go func() {
		i := subscription.Get(t)
		for m, err := range i {
			t.Must.NoError(err)
			lock.Lock()
			actual = append(actual, m.Data())
			t.Must.NoError(m.ACK())
			lock.Unlock()
		}
	}()
	t.Eventually(func(t *testcase.T) {
		blk(t.It, actual)
	})
}

func (c base[Data]) EventuallyEqual(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []Data) {
		it.Must.Equal(expected, actual)
	})
}

func (c base[Data]) EventuallyContainExactly(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it assert.It, actual []Data) {
		it.Must.ContainExactly(expected, actual)
	})
}

type Option[Data any] interface {
	option.Option[Config[Data]]
}

type Config[Data any] struct {
	MakeContext func(testing.TB) context.Context
	MakeData    func(testing.TB) Data

	SupportPublishContextCancellation bool
}

func (c *Config[Data]) Init() {
	c.MakeContext = func(t testing.TB) context.Context {
		return context.Background()
	}
	c.MakeData = spechelper.MakeValue[Data]
}

func (c Config[Data]) Configure(t *Config[Data]) {
	option.Configure(c, t)
}
