package wftesting

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"
)

type Stub struct {
	StubExecute  func(ctx context.Context, p *workflow.Process) error
	StubEvaluate func(ctx context.Context, p *workflow.Process) (bool, error)
}

var _ workflow.Definition = (*Stub)(nil)

func (stub Stub) Execute(ctx context.Context, p *workflow.Process) error {
	if stub.StubExecute != nil {
		return stub.StubExecute(ctx, p)
	}
	return nil
}

var _ workflow.Condition = (*Stub)(nil)

func (stub Stub) Evaluate(ctx context.Context, p *workflow.Process) (bool, error) {
	if stub.StubEvaluate != nil {
		return stub.StubEvaluate(ctx, p)
	}
	return true, nil
}
