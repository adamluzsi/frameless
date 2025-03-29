package iterkitcontract

import (
	"iter"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

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

	return s.AsSuite("iterator")
}
