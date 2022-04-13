package contracts_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/resources"
	inmemory2 "github.com/adamluzsi/frameless/resources/inmemory"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	testcase.RunContract(t, resources.Contract[Entity, string, string]{
		Subject: func(tb testing.TB) resources.ContractSubject[Entity, string] {
			eventLog := inmemory2.NewEventLog()
			storage := inmemory2.NewEventLogStorage[Entity, string](eventLog)
			return resources.ContractSubject[Entity, string]{
				Resource:      storage,
				MetaAccessor:  eventLog,
				CommitManager: eventLog,
			}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeEnt: func(tb testing.TB) Entity {
			t := tb.(*testcase.T)
			return Entity{Data: t.Random.String()}
		},
		MakeV: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	})
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

	resources.Contract[Entity, string, string]{
		Subject: func(tb testing.TB) resources.ContractSubject[Entity, string] {
			t, ok := tb.(*testcase.T)
			assert.Must(t).True(ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			assert.Must(t).Equal(42, vGet(t))
			assert.Must(t).Equal(42, varWithNoInit.Get(t))
			el := inmemory2.NewEventLog()
			stg := inmemory2.NewEventLogStorage[Entity, string](el)
			return resources.ContractSubject[Entity, string]{
				MetaAccessor:  el,
				CommitManager: el,
				Resource:      stg,
			}
		},
		MakeEnt: func(tb testing.TB) Entity {
			t, ok := tb.(*testcase.T)
			assert.Must(tb).True(ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			t.Must.Equal(42, vGet(t))
			t.Must.Equal(42, varWithNoInit.Get(t))
			return Entity{
				X: t.Random.String(),
				Y: t.Random.String(),
				Z: t.Random.String(),
			}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeV: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	}.Spec(s)
}
