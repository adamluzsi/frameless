package resource_test

import (
	"context"
	"github.com/adamluzsi/frameless/spechelper/resource"
	"testing"

	inmemory2 "github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase"
)

func TestContract(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	testcase.RunSuite(t, resource.Contract[Entity, string, string]{
		Subject: func(tb testing.TB) resource.ContractSubject[Entity, string] {
			eventLog := inmemory2.NewEventLog()
			storage := inmemory2.NewEventLogStorage[Entity, string](eventLog)
			return resource.ContractSubject[Entity, string]{
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
