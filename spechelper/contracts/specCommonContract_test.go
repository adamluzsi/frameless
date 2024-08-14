package contracts_test

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	eventLog := memory.NewEventLog()
	repository := memory.NewEventLogRepository[Entity, string](eventLog)

	testcase.RunSuite(t, resource.Contract[Entity, string](repository, resource.Config[Entity, string]{
		MetaAccessor:  eventLog,
		CommitManager: eventLog,
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

	el := memory.NewEventLog()
	stg := memory.NewEventLogRepository[Entity, string](el)

	resource.Contract[Entity, string](stg, resource.Config[Entity, string]{
		CRUD: crudcontracts.Config[Entity, string]{
			MakeEntity: func(tb testing.TB) Entity {
				t := mustBeTCT(tb)
				t.Must.Equal(42, vGet(t))
				t.Must.Equal(42, varWithNoInit.Get(t))
				ent := t.Random.Make(Entity{}).(Entity)
				ent.ID = ""
				return ent
			},
			ChangeEntity: func(tb testing.TB, e *Entity) {
				t := mustBeTCT(tb)
				t.Must.Equal(42, vGet(t))
				t.Must.Equal(42, varWithNoInit.Get(t))
				ogID := e.ID
				*e = t.Random.Make(Entity{}).(Entity)
				e.ID = ogID
			},
		},
	}).Spec(s)
}
