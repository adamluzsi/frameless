package resource_test

import (
	"go.llib.dev/frameless/spechelper/testent"
	"testing"

	"go.llib.dev/frameless/adapters/memory"

	"go.llib.dev/frameless/spechelper/resource"

	"github.com/adamluzsi/testcase"
)

func TestContract(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	testcase.RunSuite(t, resource.Contract[Entity, string](func(tb testing.TB) resource.ContractSubject[Entity, string] {
		t := tb.(*testcase.T)
		eventLog := memory.NewEventLog()
		repository := memory.NewEventLogRepository[Entity, string](eventLog)
		return resource.ContractSubject[Entity, string]{
			Resource:      repository,
			MetaAccessor:  eventLog,
			CommitManager: eventLog,
			MakeContext:   testent.MakeContextFunc(tb),
			MakeEntity: func() Entity {
				return Entity{Data: t.Random.String()}
			},
		}
	}))
}
