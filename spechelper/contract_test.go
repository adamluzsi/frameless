package spechelper_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

func TestContract(t *testing.T) {
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
			return fixtures.NewFactory(tb)
		},
	})
}
