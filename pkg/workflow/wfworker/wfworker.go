package wfworker

import (
	"context"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/pubsub"
)

type Worker struct {
	Runtime workflow.Runtime

	Tasks  Queue
	Errors Queue
}

type Queue interface {
	pubsub.Publisher[Job]
	pubsub.Subscriber[Job]
}

type Job struct {
	ID      JobID
	Process *workflow.Process
}

type JobID = uuid.UUID

var _ pubsub.Publisher[workflow.Definition] = (*Worker)(nil)

func (w Worker) Publish(ctx context.Context, defs ...workflow.Definition) error {
	var jobs []Job
	for _, def := range defs {
		id, err := uuid.MakeV7()
		if err != nil {
			return err
		}
		jobs = append(jobs, Job{
			ID:      id,
			Process: &workflow.Process{Definition: def},
		})
	}
	return w.Tasks.Publish(ctx, jobs...)
}

var _ tasker.Runnable = (*Worker)(nil)

func (w Worker) Run(ctx context.Context) error {
	for msg, err := range w.Tasks.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := w.handle(msg); err != nil {
			logger.Error(msg.Context(), "workflow worker encountered an error",
				logging.ErrField(err))

			w.Errors.Publish(ctx)
		}
	}
	return nil
}

func (w Worker) handle(msg pubsub.Message[Job]) (rErr error) {
	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK)
	var job = msg.Data()
	err := w.Runtime.Execute(msg.Context(), job.Process)
	if err != nil {
		return w.Errors.Publish(msg.Context(), msg.Data())
	}
	return nil
}
