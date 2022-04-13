package resources_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources"
	inmemory2 "github.com/adamluzsi/frameless/resources/inmemory"
	"github.com/adamluzsi/testcase"
)

func TestContract(t *testing.T) {
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
