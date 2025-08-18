package workflow

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/pubsub"
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

type Worker struct {
	Queue   ProcessQueue
	Runtime Runtime
}

type ProcessQueue interface {
	pubsub.Publisher[Process]
	pubsub.Subscriber[Process]
}

var _ tasker.Runnable = Worker{}

func (w Worker) Run(ctx context.Context) error {
	for msg, err := range w.Queue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := w.handle(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (w Worker) handle(ctx context.Context, msg pubsub.Message[Process]) (rErr error) {
	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
	var process = msg.Data()
	return w.Runtime.Execute(ctx, process.PDEF, process.State)
}
