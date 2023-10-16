package contracts_test

import (
	"context"
	"fmt"
	"testing"

	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	testcase.RunSuite(t, resource.Contract[Entity, string](func(tb testing.TB) resource.ContractSubject[Entity, string] {
		eventLog := memory.NewEventLog()
		repository := memory.NewEventLogRepository[Entity, string](eventLog)
		return resource.ContractSubject[Entity, string]{
			Resource:      repository,
			MetaAccessor:  eventLog,
			CommitManager: eventLog,
			MakeContext:   context.Background,
			MakeEntity: func() Entity {
				return Entity{Data: testcase.ToT(&tb).Random.String()}
			},
		}
	}))
}

func TestContracts_testcaseTNestingSupport(t *testing.T) {
	s := testcase.NewSpec(t)
	type Entity struct {
		ID      string `ext:"id"`
		X, Y, Z string
	}

	v := testcase.Let(s, func(t *testcase.T) interface{} { return 42 })
	vGet := func(t *testcase.T) int { return v.Get(t).(int) }
	varWithNoInit := testcase.Var[int]{ID: "var_with_no_init"}
	varWithNoInit.LetValue(s, 42)

	mustBeTCT := func(tb testing.TB) *testcase.T {
		t, ok := tb.(*testcase.T)
		assert.Must(tb).True(ok, assert.Message(fmt.Sprintf("expected that %T is *testcase.T", tb)))
		return t
	}

	resource.Contract[Entity, string](func(tb testing.TB) resource.ContractSubject[Entity, string] {
		t := mustBeTCT(tb)
		t.Must.Equal(42, vGet(t))
		t.Must.Equal(42, varWithNoInit.Get(t))
		el := memory.NewEventLog()
		stg := memory.NewEventLogRepository[Entity, string](el)
		return resource.ContractSubject[Entity, string]{
			MetaAccessor:  el,
			CommitManager: el,
			Resource:      stg,

			MakeContext: context.Background,
			MakeEntity: func() Entity {
				return Entity{
					X: t.Random.String(),
					Y: t.Random.String(),
					Z: t.Random.String(),
				}
			},
		}
	}).Spec(s)
}
