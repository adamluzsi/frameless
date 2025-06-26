package pubsubcontract

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Blocking[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[Data]](opts)

	b := base[Data](func(tb testing.TB) baseSubject[Data] {
		return baseSubject[Data]{
			Publisher:   publisher,
			Subscriber:  subscriber,
			MakeContext: c.MakeContext,
			MakeData:    c.MakeData,
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
				t.Must.NoError(publisher.Publish(c.MakeContext(t), c.MakeData(t)))
				publishedAt := time.Now().UTC()
				atomic.AddInt64(&publishedAtUNIXMilli, publishedAt.UnixMilli())
			}()

			var (
				receivedAt time.Time
				ackedAt    time.Time
			)
			t.Eventually(func(it *testcase.T) {
				ackedAt = sub.Get(t).AckedAt()
				it.Must.False(ackedAt.IsZero())
				receivedAt = sub.Get(t).ReceivedAt()
				it.Must.False(receivedAt.IsZero())
			})

			var publishedAt time.Time
			t.Eventually(func(t *testcase.T) {
				unixMilli := atomic.LoadInt64(&publishedAtUNIXMilli)
				t.Must.NotEmpty(unixMilli)
				publishedAt = time.UnixMilli(unixMilli).UTC()
			})

			t.Must.True(receivedAt.Before(publishedAt),
				"it was expected that the message was received before the publish was done.",
				assert.Message(fmt.Sprintf("received at - published at: %s", receivedAt.Sub(publishedAt))))

			t.Must.True(ackedAt.Before(publishedAt),
				"it was expected that acknowledging time is before the publishing time.",
				assert.Message(fmt.Sprintf("acknowledged at - published at: %s", ackedAt.Sub(publishedAt))))
		})

		if c.SupportPublishContextCancellation {
			s.Test("on context cancellation, message publishing is revoked", func(t *testcase.T) {
				sub.Get(t).Stop() // stop processing from avoiding flaky test runs

				ctx, cancel := context.WithCancel(c.MakeContext(t))
				go func() {
					// we intentionally wait a bit before cancelling out
					t.Random.Repeat(10, 100, pubsubtest.Waiter.Wait)
					cancel()
				}()

				t.Must.ErrorIs(publisher.Publish(ctx, c.MakeData(t)), context.Canceled)
				t.Must.ErrorIs(ctx.Err(), context.Canceled)

				sub.Get(t).Start(t, c.MakeContext(t))

				pubsubtest.Waiter.Wait()

				t.Must.True(sub.Get(t).AckedAt().IsZero())
			})
		}
	})

	return s.AsSuite("Blocking")
}
