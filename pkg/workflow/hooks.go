package workflow

import (
	"context"
	"time"
)

type Hooks struct {
	ProcessID        ProcessID
	EventsRepository EventRepository
}

// Defer will register a hook in the current process execution, as a cleanup.
// Defer will execute the hook at the end of the normal execution or when the application is marked as terminated/completed early on.
// When multiple Definition is set with consequent Defer calls, the execution order will be LIFO.
func (h Hooks) Defer(ctx context.Context, def Definition) error {
	return nil
}

// EventHook marks a hook registered to a specific
type EventHook struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path       Path
	VarScope   VarScope
	Definition Definition
}

// EventHookCompleted is the event completion
type EventHookCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time
	// HookID is the event id of the hook registration,
	// which makes it uniquely identifiable.
	HookID EventID
}
