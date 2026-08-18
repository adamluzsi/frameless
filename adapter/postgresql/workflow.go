package postgresql

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/guard"
)

type WorkflowLockerFactory[L guard.Unlocker] struct {
	Connection Connection
}

func (f WorkflowLockerFactory[L]) factory() LockerFactory[string, workflow.ProcessLock] {
	return LockerFactory[string, guard.NonBlockingLocker]{Connection: f.Connection}
}

func (f WorkflowLockerFactory[L]) Migrate(ctx context.Context) error {
	return f.factory().Migrate(ctx)
}

func (f WorkflowLockerFactory[L]) LockerFor(id workflow.ProcessID) guard.NonBlockingLocker {
	f.factory()
	return
}
