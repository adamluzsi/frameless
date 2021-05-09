package frameless

import (
	"context"
)

type OnePhaseCommitProtocol interface {
	// BeginTx creates a context with a transaction.
	// All statements that receive this context should be executed within the given transaction in the context.
	// After a BeginTx command will be executed in a single transaction until an explicit COMMIT or ROLLBACK is given.
	//
	// In case the resource support some form of isolation level,
	// or other ACID related property of the transaction,
	// then it is advised to prepare this information in the context before calling BeginTx.
	// e.g.:
	//   ...
	//   var err error
	//   ctx = r.ContextWithIsolationLevel(ctx, sql.LevelSerializable)
	//   ctx, err = r.BeginTx(ctx)
	//
	BeginTx(context.Context) (context.Context, error)
	// CommitTx Commit commits the current transaction.
	// All changes made by the transaction become visible to others and are guaranteed to be durable if a crash occurs.
	CommitTx(context.Context) error
	// RollbackTx rolls back the current transaction and causes all the updates made by the transaction to be discarded.
	RollbackTx(context.Context) error
}

type TwoPhaseCommitProtocol interface {
	OnePhaseCommitProtocol
	// PrepareTx communicate with the resource that the current transaction is done and should be prepared for commit later.
	//
	// Prepare transaction is not intended for use in applications or interactive sessions.
	// Its purpose is to allow an external transaction manager to perform atomic global transactions across multiple databases or other transactional resources.
	// Unless you're writing a transaction manager, you probably shouldn't be using PrepareTx.
	//
	// This command must be used on a context made with BeginTx.
	// Calling CommitTx or RollbackTx with the received context
	// must be interpreted as Two Phase Commit Protocol's Commit or Rollback action.
	PrepareTx(context.Context) (context.Context, error)
}
