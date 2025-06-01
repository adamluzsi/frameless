package iterkitcontract

import (
	"iter"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
)

func IterSeq[T any](mk func(testing.TB) iter.Seq[T]) contract.Contract {
	s := testcase.NewSpec(nil)

	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[T] {
		return mk(t)
	})

	s.Then("values can be collected from the iterator", func(t *testcase.T) {
		var vs []T
		for v := range subject.Get(t) {
			vs = append(vs, v)
		}
		assert.NotEmpty(t, vs)
	})

	s.Then("iteration can be interupted", func(t *testcase.T) {
		assert.NotPanic(t, func() {
			for _ = range mk(t) {
				break
			}
		})

		assert.NotPanic(t, func() {
			next, stop := iter.Pull(mk(t))
			_, _ = next()
			stop()
		})
	})

	thenIterationIsRepeatable(s, func(t testing.TB) iter.Seq[struct{}] {
		src := mk(t)
		return func(yield func(struct{}) bool) {
			for range src {
				if !yield(struct{}{}) {
					return
				}
			}
		}
	})

	return s.AsSuite(reflectkit.TypeOf[iter.Seq[T]]().String())
}

func IterSeq2[K, V any](mk func(testing.TB) iter.Seq2[K, V]) contract.Contract {
	s := testcase.NewSpec(nil)

	subject := testcase.Let(s, func(t *testcase.T) iter.Seq2[K, V] {
		return mk(t)
	})

	s.Then("values can be collected from the iterator", func(t *testcase.T) {
		var (
			ks []K
			vs []V
		)
		for k, v := range subject.Get(t) {
			ks = append(ks, k)
			vs = append(vs, v)
		}
		assert.NotEmpty(t, ks)
		assert.NotEmpty(t, vs)
	})

	s.Then("iteration can be interupted", func(t *testcase.T) {
		assert.NotPanic(t, func() {
			for _, _ = range mk(t) {
				break
			}
		})

		assert.NotPanic(t, func() {
			next, stop := iter.Pull2(mk(t))
			_, _, _ = next()
			stop()
		})
	})

	thenIterationIsRepeatable(s, func(t testing.TB) iter.Seq[struct{}] {
		src := mk(t)
		return func(yield func(struct{}) bool) {
			for range src {
				if !yield(struct{}{}) {
					return
				}
			}
		}
	})

	return s.AsSuite(reflectkit.TypeOf[iter.Seq2[K, V]]().String())
}

func thenIterationIsRepeatable(s *testcase.Spec, mk func(testing.TB) iter.Seq[struct{}]) {
	s.Then("iteration is repeatable", func(t *testcase.T) {
		i := mk(t)

		var hasValues bool
		t.Random.Repeat(3, 7, func() {
			var n int
			for range i {
				n++
			}
			if 0 < n {
				hasValues = true
			}
		})

		if hasValues {
			assert.Eventually(t, 3, func(t assert.It) {
				var n int
				for range i {
					n++
				}
				assert.NotEqual(t, 0, n)
			})
		} else {
			var n int
			for range i {
				n++
			}
			assert.Equal(t, 0, n)
		}
	})

}
