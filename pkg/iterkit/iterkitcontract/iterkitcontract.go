package iterkitcontract

import (
	"context"
	"iter"
	"testing"
	"time"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/sandbox"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/testcase"
)

func Iterator[V any](mk func(testing.TB) iter.Seq[V]) contract.Contract {
	s := testcase.NewSpec(nil)

	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[V] {
		return mk(t)
	})

	s.Then("values can be collected from the iterator", func(t *testcase.T) {
		var vs []V
		for v := range subject.Get(t) {
			vs = append(vs, v)
		}
		assert.NotEmpty(t, vs)
	})

	return s.AsSuite("iterator")
}

func IteratorWithRelease[V any](mk func(testing.TB) (iter.Seq[V], func() error)) contract.Contract {
	s := testcase.NewSpec(nil)

	subject, errFunc := testcase.Let2(s, func(t *testcase.T) (iter.Seq[V], func() error) {
		return mk(t)
	})

	s.Then("values can be collected from the iterator", func(t *testcase.T) {
		vs, err := iterkit.CollectErr[V](subject.Get(t), errFunc.Get(t))
		t.Must.NoError(err)
		t.Must.NotEmpty(vs)
	})

	s.Then("after release, the iterator no longer iterates", func(t *testcase.T) {
		i := subject.Get(t)
		assert.NoError(t, errFunc.Get(t)())

		var vs []V
		sandbox.Run(func() { vs = iterkit.Collect(i) })
		assert.Empty(t, vs)
	})

	s.Then("closing the iterator is possible, even multiple times, without an issue", func(t *testcase.T) {
		_ = subject.Get(t)

		t.Random.Repeat(3, 7, func() {
			assert.NoError(t, errFunc.Get(t)())
		})
	})

	s.Test("release is non-blocking", func(t *testcase.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, errFunc.Get(t)())
		})
	})

	s.When("iterator is released", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			assert.NoError(t, errFunc.Get(t)())
		})

		s.Then("no more value is iterated", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(subject.Get(t)))
		})
	})

	return s.AsSuite("iterator-with-release")
}
