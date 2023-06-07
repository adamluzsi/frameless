package contextkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/pkg/contextkit"
	"github.com/adamluzsi/testcase"
)

var _ context.Context = contextkit.Detach(context.Background())

func TestDetached(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		parent = testcase.Let(s, func(t *testcase.T) context.Context {
			return context.Background()
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) context.Context {
		return contextkit.Detach(parent.Get(t))
	})

	s.Describe(".Deadline", func(s *testcase.Spec) {
		act := func(t *testcase.T) (deadline time.Time, ok bool) {
			return subject.Get(t).Deadline()
		}

		s.Then("no deadline returned", func(t *testcase.T) {
			deadline, ok := act(t)
			t.Must.False(ok)
			t.Must.Empty(deadline)
		})

		s.When("parent deadline is reached", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
				t.Cleanup(cancelFunc)
				_, ok := ctx.Deadline()
				t.Must.True(ok)
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				deadline, ok := act(t)
				t.Must.False(ok)
				t.Must.Empty(deadline)
			})
		})
	})

	s.Describe(".Done", func(s *testcase.Spec) {
		act := func(t *testcase.T) <-chan struct{} {
			return subject.Get(t).Done()
		}

		s.Then("it is not done", func(t *testcase.T) {
			select {
			case <-act(t):
				t.FailNow()
			default:
			}
		})

		s.When("parent context is done", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				<-ctx.Done()
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				select {
				case <-act(t):
					t.FailNow()
				default:
				}
			})
		})
	})

	s.Describe(".Err", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Err()
		}

		s.Then("there is no error", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})

		s.When("parent context has an error due to context cancellation", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				t.Must.NotNil(ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})

		s.When("parent context has an error due to deadline exceed", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithDeadline(parent.Super(t), time.Now())
				cancel()
				t.Must.ErrorIs(context.DeadlineExceeded, ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})
	})
}
