package sandbox_test

import (
	"runtime"
	"testing"

	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRun(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		fn = let.Var[func()](s, nil)
	)
	act := let.Act(func(t *testcase.T) sandbox.O {
		return sandbox.Run(fn.Get(t))
	})

	s.Context("OK", func(s *testcase.Spec) {
		fn.Let(s, func(t *testcase.T) func() {
			return func() {}
		})

		s.Test("successful execution is reported", func(t *testcase.T) {
			o := act(t)

			assert.True(t, o.OK)
			assert.False(t, o.Goexit)
			assert.False(t, o.Panic)
			assert.Nil(t, o.PanicValue)
		})
	})

	s.Context("Goexit", func(s *testcase.Spec) {
		fn.Let(s, func(t *testcase.T) func() {
			return func() {
				runtime.Goexit()
			}
		})

		s.Test("non OK execution is reported with Goexit mentioned", func(t *testcase.T) {
			o := act(t)

			assert.False(t, o.OK)
			assert.True(t, o.Goexit)
			assert.False(t, o.Panic)
			assert.Nil(t, o.PanicValue)
		})
	})

	s.Context("panic", func(s *testcase.Spec) {
		val := let.Var[any](s, nil)
		fn.Let(s, func(t *testcase.T) func() {
			return func() {
				panic(val.Get(t))
			}
		})

		s.Context("nil panic value", func(s *testcase.Spec) {
			val.LetValue(s, nil)

			s.Test("panic with nil value is reported", func(t *testcase.T) {
				o := act(t)

				assert.False(t, o.OK)
				assert.False(t, o.Goexit)
				assert.True(t, o.Panic)
				assert.Nil(t, o.PanicValue)
			})
		})

		s.Context("non-nil panic value", func(s *testcase.Spec) {
			exp := let.Error(s)

			val.Let(s, func(t *testcase.T) any {
				return exp.Get(t)
			})

			s.Test("panic with the expected panic value is reported", func(t *testcase.T) {
				o := act(t)

				assert.False(t, o.OK)
				assert.False(t, o.Goexit)
				assert.True(t, o.Panic)
				assert.Equal[any](t, o.PanicValue, exp.Get(t))
			})
		})
	})

}

func BenchmarkRun(b *testing.B) {
	var fn = func() {}

	b.Run("sandbox", func(b *testing.B) {
		for b.Loop() {
			sandbox.Run(fn)
		}
	})

	b.Run("rawdog", func(b *testing.B) {
		for range b.N {
			var done = make(chan struct{})
			var pv any
			go func() {
				defer close(done)
				defer func() {
					pv = recover()
				}()
				fn()
			}()
			<-done
			_ = pv
		}
	})
}
