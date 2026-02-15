package crudcontract_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/testing/testent"
)

func TestBatcher(t *testing.T) {
	m := memory.NewMemory()
	r := memory.NewRepository[testent.Foo, testent.FooID](m)
	crudcontract.Batcher[testent.Foo, testent.FooID, crud.Batch[testent.Foo]](r).Test(t)
}
