package pubsubcontract

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

const timeout = time.Second / 2

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
				assert.Must(t).NoError(act(t))
			})

			s.When("context has an error", func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				})

				s.Then("it returns the error of the context", func(t *testcase.T) {
					assert.Must(t).ErrorIs(ctx.Get(t).Err(), act(t))
				})
			})
		})

		s.When("no events has been published published", func(s *testcase.Spec) {
			c.TryCleanup(s)

			s.Then("subscription didn't received anything", func(t *testcase.T) {
				pubsubtest.Waiter.Wait()
				assert.Must(t).Empty(sub.Get(t).Values())
			})
		})

		s.When("an event is published", func(s *testcase.Spec) {
			val := testcase.Let(s, func(t *testcase.T) Data {
				return c.subject().Get(t).MakeData(t)
			})

			c.WhenWePublish(s, val)

			s.Then("event received through the subscription", func(t *testcase.T) {
				t.Eventually(func(it *testcase.T) {
					assert.Contains(it, sub.Get(t).Values(), val.Get(t))
				})
			})
		})

		// TODO add check here for MetaAccessor
	})
}

func (c base[Data]) newSubscriptionIteratorHelper(t *testcase.T) *subscriptionHelper[Data] {
	return newSubscriptionIteratorHelper[Data](t, c.subject().Get(t).Subscriber)
}

func newSubscriptionIteratorHelper[Data any](t *testcase.T, subscriber pubsub.Subscriber[Data]) *subscriptionHelper[Data] {
	return &subscriptionHelper[Data]{id: t.Random.UUID(), Subscriber: subscriber}
}

func subscribeTo[Data any](t *testcase.T, ctx context.Context, subscriber pubsub.Subscriber[Data]) *subscriptionHelper[Data] {
	sub := newSubscriptionIteratorHelper(t, subscriber)
	sub.Start(t, ctx)
	t.Cleanup(sub.Stop)
	return sub
}

// replace me with pubsubtest.Subscribe
type subscriptionHelper[Data any] struct {
	id string

	Subscriber       pubsub.Subscriber[Data]
	HandlingDuration time.Duration

	mutex sync.Mutex
	data  []Data

	receivedAt time.Time
	ackedAt    time.Time
	cancel     func()
}

func (subH *subscriptionHelper[Data]) AckedAt() time.Time {
	subH.mutex.Lock()
	defer subH.mutex.Unlock()
	return subH.ackedAt
}

func (subH *subscriptionHelper[Data]) Values() []Data {
	subH.mutex.Lock()
	defer subH.mutex.Unlock()
	return append([]Data{}, subH.data...)
}

func (subH *subscriptionHelper[Data]) ReceivedAt() time.Time {
	subH.mutex.Lock()
	defer subH.mutex.Unlock()
	return subH.receivedAt
}

func (subH *subscriptionHelper[Data]) Start(tb testing.TB, ctx context.Context) {
	assert.Nil(tb, subH.cancel)
	ctx, cancel := context.WithCancel(ctx)
	subscription := subH.Subscriber.Subscribe(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go subH.work(tb, ctx, &wg, subscription)
	subH.cancel = func() {
		cancel()
		wg.Wait()
		subH.cancel = nil
	}
}

func (subH *subscriptionHelper[Data]) Stop() {
	if subH.cancel != nil {
		subH.cancel()
	}
}

func (subH *subscriptionHelper[Data]) work(tb testing.TB, ctx context.Context, wg *sync.WaitGroup, subscription pubsub.Subscription[Data]) {
	defer wg.Done()
	for msg, err := range subscription {
		if !subH.handle(tb, ctx, msg, err) {
			break
		}
	}
}
func (subH *subscriptionHelper[Data]) handle(tb testing.TB, ctx context.Context, msg pubsub.Message[Data], err error) bool {
	var should = assert.Should(tb)
	if err != nil {
		should.AnyOf(func(a *assert.A) {
			// TODO: survey which behaviour is more natural
			a.Test(func(t testing.TB) { assert.ErrorIs(t, ctx.Err(), err) })
			a.Test(func(t testing.TB) { assert.NoError(t, err) })
		})
		return true
	}
	subH.mutex.Lock()
	defer subH.mutex.Unlock()
	if msg == nil {
		should.True(msg != nil, "msg should not be nil")
		return false
	}
	subH.receivedAt = time.Now().UTC()
	if 0 < subH.HandlingDuration {
		time.Sleep(subH.HandlingDuration)
	}
	pubsubtest.Waiter.Wait()
	subH.data = append(subH.data, msg.Data())
	subH.ackedAt = time.Now().UTC()
	pubsubtest.Waiter.Wait()
	should.NoError(msg.ACK())
	return true
}

func (subH *subscriptionHelper[Data]) assertEmpty(t *testcase.T, msg ...assert.Message) {
	t.Helper()
	t.Random.Repeat(3, 7, func() {
		t.Helper()

		assert.Empty(t, subH.Values(), msg...)
	})
}

func (c base[Data]) GivenWeHaveSubscription(s *testcase.Spec) testcase.Var[*subscriptionHelper[Data]] {
	return testcase.Let(s, func(t *testcase.T) *subscriptionHelper[Data] {
		return subscribeTo(t, c.subject().Get(t).MakeContext(t), c.subject().Get(t).Subscriber)
	}).EagerLoading(s)
}

func (c base[Data]) GivenWeHadSubscriptionBefore(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		t.Log("given the subscription was at least once made")
		sub := c.newSubscriptionIteratorHelper(t)
		sub.Start(t, c.subject().Get(t).MakeContext(t))
		sub.Stop()
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
			assert.Must(t).NoError(c.subject().Get(t).Publisher.Publish(c.subject().Get(t).MakeContext(t), v.Get(t)))
			pubsubtest.Waiter.Wait()
		}
	})
}

func (c base[Data]) EventuallyIt(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], blk func(it testing.TB, actual []Data)) {
	var (
		actual []Data
		lock   sync.Mutex
	)
	go func() {
		i := subscription.Get(t)
		for m, err := range i {
			assert.Must(t).NoError(err)
			lock.Lock()
			actual = append(actual, m.Data())
			assert.Must(t).NoError(m.ACK())
			lock.Unlock()
		}
	}()
	t.Eventually(func(t *testcase.T) {
		blk(t, actual)
	})
}

func (c base[Data]) EventuallyEqual(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it testing.TB, actual []Data) {
		assert.Equal(it, expected, actual)
	})
}

func (c base[Data]) EventuallyContainExactly(t *testcase.T, subscription testcase.Var[pubsub.Subscription[Data]], expected []Data) {
	c.EventuallyIt(t, subscription, func(it testing.TB, actual []Data) {
		assert.ContainsExactly(it, expected, actual)
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
	*t = reflectkit.MergeStruct(*t, c)
}
