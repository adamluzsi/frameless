package pubsubtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/port/comproto"

	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/assert"
)

var Waiter = assert.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

var Eventually = assert.Retry{
	Strategy: &Waiter,
}

type AsyncResults[Data any] struct {
	values         []Data
	mutex          sync.Mutex
	finish         func()
	lastReceivedAt time.Time
	subscription   pubsub.Subscription[Data]
}

func (r *AsyncResults[Data]) Subscription() pubsub.Subscription[Data] {
	return r.subscription
}

func (r *AsyncResults[Data]) Add(d Data) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.lastReceivedAt = time.Now().UTC()
	r.values = append(r.values, d)
}

func (r *AsyncResults[Data]) Values() []Data {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]Data{}, r.values...)
}

func (r *AsyncResults[Data]) ReceivedAt() time.Time {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.lastReceivedAt
}

func (r *AsyncResults[Data]) Eventually(tb testing.TB, blk func(testing.TB, []Data)) {
	tb.Helper()
	Eventually.Assert(tb, func(it testing.TB) { blk(it, r.Values()) })
}

func (r *AsyncResults[Data]) Finish() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.finish == nil {
		return
	}
	r.finish()
}

func Subscribe[Data any](tb testing.TB, sub pubsub.Subscriber[Data], ctx context.Context) *AsyncResults[Data] {
	var r AsyncResults[Data]
	c := consumer[Data]{Subscriber: sub, OnData: r.Add}
	r.subscription = c.Start(tb, ctx)
	tb.Cleanup(c.Stop)
	r.finish = c.Stop
	return &r
}

type consumer[Data any] struct {
	Subscriber pubsub.Subscriber[Data]
	OnData     func(Data)
	cancel     func()
}

func (sih *consumer[Data]) Start(tb testing.TB, ctx context.Context) pubsub.Subscription[Data] {
	assert.Nil(tb, sih.cancel)
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	sub := sih.Subscriber.Subscribe(ctx)
	go sih.wrk(tb, ctx, &wg, sub)
	sih.cancel = func() {
		cancel()
		wg.Wait()
		sih.cancel = nil
	}
	return sub
}

func (sih *consumer[Data]) Stop() {
	if sih.cancel != nil {
		sih.cancel()
	}
}

func (sih *consumer[Data]) wrk(
	tb testing.TB,
	ctx context.Context,
	wg *sync.WaitGroup,
	itr pubsub.Subscription[Data],
) {
	defer wg.Done()
	for v, err := range itr {
		if err != nil {
			assert.Should(tb).AnyOf(func(a *assert.A) {
				// TODO: survey which behaviour is more natural
				a.Test(func(t testing.TB) { assert.ErrorIs(t, ctx.Err(), err) })
				a.Test(func(t testing.TB) { assert.NoError(t, err) })
			}, assert.Message(fmt.Sprintf("error: %#v", err)))
			continue
		}
		assert.Should(tb).NoError(func(msg pubsub.Message[Data]) (rErr error) {
			defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
			if sih.OnData != nil {
				sih.OnData(msg.Data())
			}
			return nil
		}(v))
	}
}
