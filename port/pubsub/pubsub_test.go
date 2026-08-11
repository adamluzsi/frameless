package pubsub_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestMakeMessage(t *testing.T) {
	s := testcase.NewSpec(t)

	type Data struct{ V string }

	var (
		ctx  = let.Context(s)
		data = let.Var(s, func(t *testcase.T) Data {
			return Data{V: t.Random.String()}
		})

		ackCount    = let.VarOf(s, 0)
		ackErr      = let.VarOf[error](s, nil)
		ackReceived = let.VarOf[pubsub.Message[Data]](s, nil)
		ack         = let.Var(s, func(t *testcase.T) func(pubsub.Message[Data]) error {
			return func(msg pubsub.Message[Data]) error {
				ackCount.Set(t, ackCount.Get(t)+1)
				ackReceived.Set(t, msg)
				return ackErr.Get(t)
			}
		})

		nackCount    = let.VarOf(s, 0)
		nackErr      = let.VarOf[error](s, nil)
		nackReceived = let.VarOf[pubsub.Message[Data]](s, nil)
		nack         = let.Var(s, func(t *testcase.T) func(pubsub.Message[Data]) error {
			return func(msg pubsub.Message[Data]) error {
				nackCount.Set(t, nackCount.Get(t)+1)
				nackReceived.Set(t, msg)
				return nackErr.Get(t)
			}
		})
	)

	// subject is the system under test: the message produced by MakeMessage.
	// It is memoized per test, so calls made against it (ACK/NACK) share state.
	subject := let.Var(s, func(t *testcase.T) pubsub.Message[Data] {
		return pubsub.MakeMessage[Data](ctx.Get(t), data.Get(t), ack.Get(t), nack.Get(t))
	})

	s.Then("the result implements pubsub.Message", func(t *testcase.T) {
		var _ pubsub.Message[Data] = subject.Get(t)
	})

	s.Describe(".Context", func(s *testcase.Spec) {
		act := func(t *testcase.T) context.Context {
			return subject.Get(t).Context()
		}

		s.Then("it returns the context the message was made with", func(t *testcase.T) {
			assert.Equal(t, ctx.Get(t), act(t))
		})

		s.When("the context is nil", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				return nil
			})

			s.Then("it falls back to a non-nil background context", func(t *testcase.T) {
				assert.NotNil(t, act(t))
				assert.Equal(t, context.Background(), act(t))
			})
		})
	})

	s.Describe(".Data", func(s *testcase.Spec) {
		act := func(t *testcase.T) Data {
			return subject.Get(t).Data()
		}

		s.Then("it returns the data the message was made with", func(t *testcase.T) {
			assert.Equal(t, data.Get(t), act(t))
		})
	})

	s.Describe(".ACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).ACK()
		}

		s.Then("it reports no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("it invokes the ack function once", func(t *testcase.T) {
			assert.Equal(t, 0, ackCount.Get(t), "guard: ack must not be called before ACK")
			assert.NoError(t, act(t))
			assert.Equal(t, 1, ackCount.Get(t))
		})

		s.Then("it does not invoke the nack function", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, 0, nackCount.Get(t))
		})

		s.Then("it passes the message itself to the ack function", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NotNil(t, ackReceived.Get(t))
			assert.True(t, ackReceived.Get(t) == subject.Get(t),
				"the ack function must receive the message being settled")
		})

		s.Then("it is idempotent: repeated calls invoke the ack function only once", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.Equal(t, 1, ackCount.Get(t))
		})

		s.When("the ack function returns an error", func(s *testcase.Spec) {
			expErr := let.Error(s)
			ackErr.Let(s, expErr.Get)

			s.Then("the error is propagated", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})

			s.Then("the error is cached and returned on every call, without re-invoking ack", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
				assert.ErrorIs(t, act(t), expErr.Get(t))
				assert.Equal(t, 1, ackCount.Get(t))
			})
		})

		s.When("no ack function was provided", func(s *testcase.Spec) {
			ack.Let(s, func(t *testcase.T) func(pubsub.Message[Data]) error {
				return nil
			})

			s.Then("it is a no-op that reports no error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("the message was already finalized by a NACK", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				_ = subject.Get(t).NACK() // arrange: settle the message through NACK first
			})

			s.Then("ACK does not invoke the ack function", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, 0, ackCount.Get(t))
			})

			s.And("that prior NACK returned an error", func(s *testcase.Spec) {
				expErr := let.Error(s)
				nackErr.Let(s, expErr.Get)

				s.Then("ACK returns the cached NACK error and still does not invoke ack", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					assert.Equal(t, 0, ackCount.Get(t))
				})
			})
		})
	})

	s.Describe(".NACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).NACK()
		}

		s.Then("it reports no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("it invokes the nack function once", func(t *testcase.T) {
			assert.Equal(t, 0, nackCount.Get(t), "guard: nack must not be called before NACK")
			assert.NoError(t, act(t))
			assert.Equal(t, 1, nackCount.Get(t))
		})

		s.Then("it does not invoke the ack function", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, 0, ackCount.Get(t))
		})

		s.Then("it passes the message itself to the nack function", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NotNil(t, nackReceived.Get(t))
			assert.True(t, nackReceived.Get(t) == subject.Get(t),
				"the nack function must receive the message being settled")
		})

		s.Then("it is idempotent: repeated calls invoke the nack function only once", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.Equal(t, 1, nackCount.Get(t))
		})

		s.When("the nack function returns an error", func(s *testcase.Spec) {
			expErr := let.Error(s)
			nackErr.Let(s, expErr.Get)

			s.Then("the error is propagated", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})

			s.Then("the error is cached and returned on every call, without re-invoking nack", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
				assert.ErrorIs(t, act(t), expErr.Get(t))
				assert.Equal(t, 1, nackCount.Get(t))
			})
		})

		s.When("no nack function was provided", func(s *testcase.Spec) {
			nack.Let(s, func(t *testcase.T) func(pubsub.Message[Data]) error {
				return nil
			})

			s.Then("it is a no-op that reports no error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("the message was already finalized by an ACK", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				_ = subject.Get(t).ACK() // arrange: settle the message through ACK first
			})

			s.Then("NACK does not invoke the nack function", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, 0, nackCount.Get(t))
			})

			s.And("that prior ACK returned an error", func(s *testcase.Spec) {
				expErr := let.Error(s)
				ackErr.Let(s, expErr.Get)

				s.Then("NACK returns the cached ACK error and still does not invoke nack", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					assert.Equal(t, 0, nackCount.Get(t))
				})
			})
		})
	})
}

func TestZeroMessage(t *testing.T) {
	s := testcase.NewSpec(t)

	type Data struct{ V string }

	subject := let.Var(s, func(t *testcase.T) pubsub.Message[Data] {
		return pubsub.ZeroMessage[Data]()
	})

	s.Then("the result implements pubsub.Message", func(t *testcase.T) {
		var _ pubsub.Message[Data] = subject.Get(t)
	})

	s.Describe(".Context", func(s *testcase.Spec) {
		act := func(t *testcase.T) context.Context {
			return subject.Get(t).Context()
		}

		s.Then("it is a non-nil background context", func(t *testcase.T) {
			assert.NotNil(t, act(t))
			assert.Equal(t, context.Background(), act(t))
		})
	})

	s.Describe(".Data", func(s *testcase.Spec) {
		act := func(t *testcase.T) Data {
			return subject.Get(t).Data()
		}

		s.Then("it is the zero value", func(t *testcase.T) {
			assert.Equal(t, *new(Data), act(t))
		})
	})

	s.Describe(".ACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).ACK()
		}

		s.Then("it is a no-op that reports no error, even when called repeatedly", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
		})
	})

	s.Describe(".NACK", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).NACK()
		}

		s.Then("it is a no-op that reports no error, even when called repeatedly", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
		})
	})
}
