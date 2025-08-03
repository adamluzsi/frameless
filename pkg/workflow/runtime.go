package workflow

import (
	"context"
	"fmt"
)

// Runtime is a workflow engine runtime instance that contains the runtime dependencies for executing process definitions.
type Runtime struct {
	Participants    Participants
	TemplateFuncMap TemplateFuncMap
}

func (r Runtime) Context(ctx context.Context) context.Context {
	ctx = ContextWithParticipants(ctx, r.Participants)
	ctx = ContextWithFuncMap(ctx, r.TemplateFuncMap)
	return ctx
}

func (r Runtime) Execute(ctx context.Context, pdef Definition, state *State) error {
	ctx = r.Context(ctx)
	if pdef == nil {
		return fmt.Errorf("nil workflow process definition was received")
	}
	if state == nil {
		return fmt.Errorf("nil workflow process state was received")
	}
	if err := pdef.Validate(ctx); err != nil {
		return err
	}
	return pdef.Execute(ctx, state)
}
