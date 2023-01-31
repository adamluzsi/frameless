package pubsubcontracts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adamluzsi/testcase"
)

type Blocking[V any] struct {
	MakeSubject func(testing.TB) PubSub[V]
	MakeContext func(testing.TB) context.Context
	MakeV       func(testing.TB) V
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

		s.When("a subscription is made", func(s *testcase.Spec) {
			sub := b.GivenWeHaveSubscription(s)

			s.Before(func(t *testcase.T) {
				d := 42 * time.Millisecond
				t.Logf("processing a message takes significant time (%s)", d.String())
				sub.Get(t).HandlingDuration = d
			})

			s.And("a message is published", func(s *testcase.Spec) {
				publishedAt := testcase.Let(s, func(t *testcase.T) time.Time {
					t.Must.NoError(b.subject().Get(t).Publish(c.MakeContext(t), c.MakeV(t)))
					return time.Now().UTC()
				}).EagerLoading(s)

				s.Then("publish will block until subscriber acknowledge the message by finishing the handling", func(t *testcase.T) {
					t.Must.True(publishedAt.Get(t).After(sub.Get(t).ReceivedAt))
				})
			})
		})
	})
}

func (c Blocking[V]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c Blocking[V]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
