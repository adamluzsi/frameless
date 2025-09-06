package pubsubcontract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Blocking[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

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
			sub.Get(t).HandlingDuration = time.Millisecond

			var m synckit.Map[string, time.Time]
			const PublishedAt = "publishedAt"
			const ReceivedAt = "receivedAt"
			const AckedAt = "ackedAt"

			go func() {
				ctx, cancel := contextkit.Merge(c.MakeContext(t), t.Context())
				defer cancel()
				t.Must.NoError(publisher.Publish(ctx, c.MakeData(t)))
				m.Set(PublishedAt, time.Now())
			}()

			t.Eventually(func(it *testcase.T) {
				ackedAt := sub.Get(t).AckedAt()
				assert.False(it, ackedAt.IsZero())
				receivedAt := sub.Get(t).ReceivedAt()
				assert.False(it, receivedAt.IsZero())
				m.Set(AckedAt, ackedAt)
				m.Set(ReceivedAt, receivedAt)
			})

			t.Eventually(func(t *testcase.T) {
				_, ok := m.Lookup(PublishedAt)
				assert.True(t, ok)
			})

			var (
				publishedAt = m.Get(PublishedAt)
				receivedAt  = m.Get(ReceivedAt)
				ackedAt     = m.Get(AckedAt)
			)

			assert.True(t, receivedAt.Before(publishedAt),
				"it was expected that the message was received before the publish was done.",
				assert.Message(fmt.Sprintf("received at - published at: %s", receivedAt.Sub(publishedAt))))

			assert.True(t, ackedAt.Before(publishedAt),
				"it was expected that acknowledging time is before the publishing time.",
				assert.MessageF("\n\tacknowledged at %s\n\tpublished at: %s\n\tdiff: %d",
					ackedAt.Format(time.RFC3339Nano),
					publishedAt.Format(time.RFC3339Nano),
					publishedAt.Sub(publishedAt),
				))
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

				assert.True(t, sub.Get(t).AckedAt().IsZero())
			})
		}
	})

	return s.AsSuite("Blocking")
}
