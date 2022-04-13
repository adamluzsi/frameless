package doubles

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type StubOnePhaseCommitProtocol struct {
	frameless.OnePhaseCommitProtocol
	BeginTxFunc    func(ctx context.Context) (context.Context, error)
	CommitTxFunc   func(ctx context.Context) error
	RollbackTxFunc func(ctx context.Context) error
}

func (spy *StubOnePhaseCommitProtocol) BeginTx(ctx context.Context) (context.Context, error) {
	if spy.BeginTxFunc != nil {
		return spy.BeginTxFunc(ctx)
	}
	return spy.OnePhaseCommitProtocol.BeginTx(ctx)
}

func (spy *StubOnePhaseCommitProtocol) CommitTx(ctx context.Context) error {
	if spy.CommitTxFunc != nil {
		return spy.CommitTxFunc(ctx)
	}
	return spy.OnePhaseCommitProtocol.CommitTx(ctx)
}

func (spy *StubOnePhaseCommitProtocol) RollbackTx(ctx context.Context) error {
	if spy.RollbackTxFunc != nil {
		return spy.RollbackTxFunc(ctx)
	}
	return spy.OnePhaseCommitProtocol.RollbackTx(ctx)
}
