package pubsubcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"sync/atomic"
	"testing"
	"time"
)

type Blocking[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V

	RollbackOnPublishCancellation bool
}

func (c Blocking[V]) Spec(s *testcase.Spec) {
	b := pubsubBase[V]{
		MakeSubject: c.MakeSubject,
		MakeContext: c.MakeContext,
		MakeValue:   c.MakeV,
	}
	b.Spec(s)

	s.Context(fmt.Sprintf("%s is blocking pub/sub", b.getPubSubTypeName()), func(s *testcase.Spec) {
		b.WhenIsEmpty(s)

		sub := b.GivenWeHaveSubscription(s)

		s.Test("publish will block until a subscriber acknowledged the published message", func(t *testcase.T) {
			d := 42 * 10 * time.Millisecond
			t.Logf("processing a message takes significant time (%s)", d.String())
			sub.Get(t).HandlingDuration = d

			var publishedAtUNIXMilli int64
			go func() {
				t.Must.NoError(b.subject().Get(t).Publish(c.MakeContext(t), c.MakeV(t)))
				atomic.AddInt64(&publishedAtUNIXMilli, time.Now().UTC().UnixMilli())
			}()

			var ackedAt time.Time
			t.Eventually(func(it assert.It) {
				ackedAt = sub.Get(t).AckedAt()
				it.Must.False(ackedAt.IsZero())
			})

			var publishedAt time.Time
			t.Eventually(func(t assert.It) {
				unixMilli := atomic.LoadInt64(&publishedAtUNIXMilli)
				t.Must.NotEmpty(unixMilli)
				publishedAt = time.UnixMilli(unixMilli)
			})

			t.Must.True(ackedAt.Before(publishedAt),
				"it was expected that acknowledging time is before the publishing time")
		})

		if c.RollbackOnPublishCancellation {
			s.Test("on context cancellation, message publishing is rewoked", func(t *testcase.T) {
				sub.Get(t).Stop() // stop processing from avoiding flaky test runs

				ctx, cancel := context.WithCancel(c.MakeContext(t))
				go func() {
					t.Random.Repeat(10, 100, pubsubtest.Waiter.Wait)
					cancel()
				}()

				t.Must.ErrorIs(ctx.Err(), b.subject().Get(t).Publish(ctx, c.MakeV(t)))

				sub.Get(t).Start(t, c.MakeContext(t))

				pubsubtest.Waiter.Wait()

				t.Must.True(sub.Get(t).AckedAt().IsZero())
			})
		}
	})
}

func (c Blocking[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Blocking[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
