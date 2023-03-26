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

type Blocking[Data any] func(testing.TB) BlockingSubject[Data]

type BlockingSubject[Data any] struct {
	PubSub PubSub[Data]

	MakeContext func() context.Context
	MakeData    func() Data

	RollbackOnPublishCancellation bool
}

func (c Blocking[Data]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) BlockingSubject[Data] { return c(t) })

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		sub := subject.Get(testcase.ToT(&tb))
		return baseSubject[Data]{
			PubSub:      sub.PubSub,
			MakeContext: sub.MakeContext,
			MakeData:    sub.MakeData,
		}
	})
	b.Spec(s)

	s.Context("blocking pub/sub", func(s *testcase.Spec) {
		b.TryCleanup(s)

		sub := b.GivenWeHaveSubscription(s)

		s.Test("publish will block until a subscriber acknowledged the published message", func(t *testcase.T) {
			sub.Get(t).HandlingDuration = 42 * time.Millisecond

			var publishedAtUNIXMilli int64
			go func() {
				t.Must.NoError(b.subject().Get(t).PubSub.Publish(subject.Get(t).MakeContext(), subject.Get(t).MakeData()))
				publishedAt := time.Now().UTC()
				atomic.AddInt64(&publishedAtUNIXMilli, publishedAt.UnixMilli())
			}()

			var (
				receivedAt time.Time
				ackedAt    time.Time
			)
			t.Eventually(func(it assert.It) {
				ackedAt = sub.Get(t).AckedAt()
				it.Must.False(ackedAt.IsZero())
				receivedAt = sub.Get(t).ReceivedAt()
				it.Must.False(receivedAt.IsZero())
			})

			var publishedAt time.Time
			t.Eventually(func(t assert.It) {
				unixMilli := atomic.LoadInt64(&publishedAtUNIXMilli)
				t.Must.NotEmpty(unixMilli)
				publishedAt = time.UnixMilli(unixMilli).UTC()
			})

			t.Must.True(receivedAt.Before(publishedAt),
				"it was expected that the message was received before the publish was done.",
				fmt.Sprintf("received at - published at: %s", receivedAt.Sub(publishedAt)))

			t.Must.True(ackedAt.Before(publishedAt),
				"it was expected that acknowledging time is before the publishing time.",
				fmt.Sprintf("acknowledged at - published at: %s", ackedAt.Sub(publishedAt)))
		})

		s.Test("on context cancellation, message publishing is revoked", func(t *testcase.T) {
			if !subject.Get(t).RollbackOnPublishCancellation {
				t.Skip()
			}

			sub.Get(t).Stop() // stop processing from avoiding flaky test runs

			ctx, cancel := context.WithCancel(subject.Get(t).MakeContext())
			go func() {
				t.Random.Repeat(10, 100, pubsubtest.Waiter.Wait)
				cancel()
			}()

			t.Must.ErrorIs(ctx.Err(), b.subject().Get(t).PubSub.Publish(ctx, subject.Get(t).MakeData()))

			sub.Get(t).Start(t, subject.Get(t).MakeContext())

			pubsubtest.Waiter.Wait()

			t.Must.True(sub.Get(t).AckedAt().IsZero())
		})
	})
}

func (c Blocking[Data]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Blocking[Data]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
