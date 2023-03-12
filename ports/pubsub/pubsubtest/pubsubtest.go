package pubsubtest

import (
	"context"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/testcase/assert"
	"sync"
	"testing"
	"time"
)

var Waiter = assert.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

var Eventually = assert.Eventually{
	RetryStrategy: &Waiter,
}

type AsyncResults[Data any] struct {
	tb     testing.TB
	values []Data
	mutex  sync.Mutex
	finish func()

	lastReceivedAt time.Time
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
	Eventually.Assert(tb, func(it assert.It) { blk(it, r.Values()) })
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
	c.Start(tb, ctx)
	tb.Cleanup(c.Stop)
	r.finish = c.Stop
	return &r
}

type consumer[Data any] struct {
	Subscriber pubsub.Subscriber[Data]
	OnData     func(Data)
	cancel     func()
}

func (sih *consumer[Data]) Start(tb testing.TB, ctx context.Context) {
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

func (sih *consumer[Data]) Stop() {
	if sih.cancel != nil {
		sih.cancel()
	}
}

func (sih *consumer[Data]) wrk(tb testing.TB, ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	iter := sih.Subscriber.Subscribe(ctx)
	for iter.Next() {
		assert.Should(tb).NoError(func(msg pubsub.Message[Data]) (rErr error) {
			defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
			if sih.OnData != nil {
				sih.OnData(msg.Data())
			}
			return nil
		}(iter.Value()))
	}
	assert.Should(tb).AnyOf(func(a *assert.AnyOf) {
		// TODO: survey which behaviour is more natural
		a.Test(func(t assert.It) { t.Must.ErrorIs(ctx.Err(), iter.Err()) })
		a.Test(func(t assert.It) { t.Must.NoError(iter.Err()) })
	})
}
