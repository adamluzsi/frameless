package resource_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"

	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/testcase"
)

func TestContract(t *testing.T) {
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
