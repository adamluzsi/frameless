package wfworker

import (
	"context"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/pubsub"
)

type Worker struct {
	Runtime workflow.Runtime
	Queue   Queue
}

type Queue interface {
	pubsub.Publisher[Job]
	pubsub.Subscriber[Job]
}

type Job struct {
	Definition workflow.Definition
	Process    *workflow.Process
	Status     JobStatus
}

type JobStatus string

const (
	// Pending: Job is scheduled but waiting for resources or a trigger (e.g., immediate or timed start).
	Pending JobStatus = "pending"
	// Active: Job is executing; modifications usually blocked.
	Active JobStatus = "active"
	// Finished: Job completed without errors.
	Finished JobStatus = "finished"
	// Failed: Job terminated due to exceptions; often retryable or reviewable.
	Failed JobStatus = "failed"
	// Cancelled: Job manually stopped or aborted before/during execution.
	Cancelled JobStatus = "cancelled"
)

var _ = enum.Register[JobStatus](Pending, Active, Finished, Failed, Cancelled)

var _ pubsub.Publisher[workflow.Definition] = (*Worker)(nil)

func (w Worker) Publish(ctx context.Context, defs ...workflow.Definition) error {
	return w.Queue.Publish(ctx, slicekit.Map(defs, func(def workflow.Definition) Job {
		return Job{
			Process: &workflow.Process{},
			Status:  Pending,
		}
	})...)
}

var _ tasker.Runnable = (*Worker)(nil)

func (w Worker) Run(ctx context.Context) error {
	for msg, err := range w.Queue.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if msg.Data().Status == Finished {
			msg.ACK()
			continue
		}

		job := msg.Data()

		err := w.Runtime.Execute(msg.Context(), job.Definition, job.Process)
	}
	return nil
}
