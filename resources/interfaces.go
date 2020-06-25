package resources

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type Creator interface {
	// Create takes a ptr to a entity<T> and store it into the resource.
	// It also updates the entity<T> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Create error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Create(ctx context.Context, ptr interface{}) error
}

type Finder interface {
	// FindByID will link an entity that is found in the resource to the received ptr,
	// and report back if it succeeded finding the entity in the resource.
	// It also reports if there was an unexpected exception during the execution.
	// It was an intentional decision to not use error to represent "not found" case,
	// but tell explicitly this information in the form of return bool value.
	FindByID(ctx context.Context, ptr interface{}, id string) (_found bool, _err error)
	// FindAll will return all entity that has <T> type
	FindAll(ctx context.Context, T interface{}) frameless.Iterator
}

type Updater interface {
	// Update will takes a ptr that points to an entity
	// and update the corresponding stored entity with the received entity field values
	Update(ctx context.Context, ptr interface{}) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter interface {
	// DeleteByID will remove a <T> type entity from the storage by a given ID
	DeleteByID(ctx context.Context, T interface{}, id string) error
	// DeleteAll will erase all entity from the resource that has <T> type
	DeleteAll(ctx context.Context, T interface{}) error
}

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
	// Commit commits the current transaction.
	// All changes made by the transaction become visible to others and are guaranteed to be durable if a crash occurs.
	CommitTx(context.Context) error
	// Rollback rolls back the current transaction and causes all the updates made by the transaction to be discarded.
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
