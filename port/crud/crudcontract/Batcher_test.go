package crudcontract_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
)

func TestBatcher(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("memory.Repository", func(s *testcase.Spec) {
		m := memory.NewMemory()
		r := memory.NewRepository[testent.Foo, testent.FooID](m)
		crudcontract.Batcher[testent.Foo, testent.FooID](r).Spec(s)
	})

	s.Context("memory.EventLogRepository", func(s *testcase.Spec) {
		m := memory.NewEventLog()
		r := memory.NewEventLogRepository[testent.Foo, testent.FooID](m)
		crudcontract.Batcher[testent.Foo, testent.FooID](r).Spec(s)
	})
}
