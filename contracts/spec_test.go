package contracts_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func TestContracts(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	T := Entity{}
	testcase.RunContract(t, spechelper.Contract{
		T: T,
		V: string(""),
		Subject: func(tb testing.TB) spechelper.ContractSubject {
			eventLog := inmemory.NewEventLog()
			storage := inmemory.NewEventLogStorage(T, eventLog)
			return spechelper.ContractSubject{
				CRUD:                   storage,
				MetaAccessor:           eventLog,
				OnePhaseCommitProtocol: eventLog,
			}
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			ff := fixtures.NewFactory(tb)
			if v := ff.Fixture(T, context.Background()).(Entity); v == T {
				tb.Fatal()
			}
			return ff
		},
	})
}

func TestContracts_testcaseTNestingSupport(t *testing.T) {
	s := testcase.NewSpec(t)
	type Entity struct {
		ID      string `ext:"id"`
		X, Y, Z string
	}

	v := s.Let(`example var`, func(t *testcase.T) interface{} { return 42 })
	vGet := func(t *testcase.T) int { return v.Get(t).(int) }
	varWithNoInit := testcase.Var{Name: "var_with_no_init"}
	varWithNoInit.LetValue(s, 42)

	spechelper.Contract{T: Entity{}, V: "string",
		Subject: func(tb testing.TB) spechelper.ContractSubject {
			t, ok := tb.(*testcase.T)
			require.True(t, ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			require.Equal(t, 42, vGet(t))
			require.Equal(t, 42, varWithNoInit.Get(t).(int))
			el := inmemory.NewEventLog()
			stg := inmemory.NewEventLogStorage(Entity{}, el)
			return spechelper.ContractSubject{
				MetaAccessor:           el,
				OnePhaseCommitProtocol: el,
				CRUD:                   stg,
			}
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			t, ok := tb.(*testcase.T)
			require.True(t, ok, fmt.Sprintf("expected that %T is *testcase.T", tb))
			require.Equal(t, 42, vGet(t))
			require.Equal(t, 42, varWithNoInit.Get(t).(int))
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)
}
