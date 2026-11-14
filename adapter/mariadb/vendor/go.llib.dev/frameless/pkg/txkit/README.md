# `txkit` Package

The `txkit` package is designed to help you manage resources that lack native commit protocols by mimicking transaction-like behaviour. It simplifies transaction management at the use-case code level, proving especially valuable in stateful systems, such as APIs without built-in transactional capabilities.

When an error occurs during an operation, the `txkit` package ensures all stateful changes are undone in a Last-In-First-Out (LIFO) manner, similar to how `defer` works at the function scope, but `txkit` operates at the use-case scope. While it doesn't replace proper transaction support, it can handle many scenarios where extensive rollback logic implementations or manual intervention were necessary, making human interaction almost negligible.

```go
package mypkg

import (
	"context"
	"go.llib.dev/frameless/pkg/txkit"
)

func MyUseCase(ctx context.Context) (returnErr error) {

	// Begins a new transaction, returning it as a context.
	//
	// If there was a transaction present in the received context, 
	// it automatically nests the transaction level.
	//
	// This means that this function can operate on its own,
	// or can dynamically join into a bigger use-case 
	// where multiple components are touched to achieve a high-level goal.
	ctx, err := txs.Begin(ctx)
	if err != nil {
		return err
	}

	// the deferred Finish call will finish the current transaction.
	// If the function returns without an error, txs.Commit is executed on the current transaction. 
	// In case of an error, Rollback is triggered.
	defer txs.Finish(&returnErr, ctx)

	// The happy path where we make changes to the system state.
	// This could be anything from an API call to a file system change.
	if err := MyActionWhichMutateTheSystemState(ctx); err != nil {
		return err
	}

	// Describes what to do in case of an error, ensuring a rollback for the stateful changes above.
	txs.OnRollback(ctx, func(ctx context.Context) error {
		return UndoActionForMyPreviousActionWhichMutatedTheSystemState(ctx)
	})

	return nil
}

```

## Challenges without `txkit`

In systems without transactional support, managing rollback logic becomes complex as the number of actions involving external resources increases. For instance, if you create entities `A`, `B`, `C`, and `D`, and encounter an error during the creation of `C`, you must undo `A` and `B`. An error during the creation of `D` means rolling back `A`, `B`, and `C`. As actions become more intricate, maintaining an effective rollback mechanism becomes challenging.

## How `txkit` Simplifies

The `txkit` package simplifies this by allowing you to register an OnRollback callback after each successful action that changes the system state. When an error occurs, rolling back the current `txkit` transaction executes all prepared rollback steps in a Last-In-First-Out (LIFO) order, similar to defer functionality. This ensures the system state is restored to stability.

## Reduced Mental Load

With `txkit`, your code focuses on individual entities and their actions, reducing the mental load needed for rollbacks. For example, the code for creating entity `C` involves starting a transaction, performing the action (creating `C`), defining rollback steps (deleting `C`), and finishing the transaction with either Commit or Rollback in case of an error.

## Nested Transactions

The package supports nested transactions seamlessly, automatically joining an existing transaction if present in the context. This allows smaller transactions to combine into larger units without code changes.

## Transaction Resource Identity

`Manager.ResourceID` scopes context transactions by **resource ID and `TX` type**.
Use a stable, distinct ID for each independent transactional resource. Managers
with matching IDs and transaction types share and nest transactions, including
when manager values are copied or reconstructed. Different IDs can have separate
transactions in the same context chain; lookup, query routing, commit and rollback
select only that manager's scope. Normal parent-context cancellation still applies.

An empty ID defaults to `fmt.Sprintf("%p", m.R)`: wrappers around the same
resource pointer and `TX` type share a scope, while distinct pointers remain
isolated. An explicit ID can share a logical resource across different handles.
Do not change the ID or resource pointer while transaction contexts are in use.

`flsql.ConnectionAdapter` derives the ID from `fmt.Sprintf("%p", c.DB)`: wrappers
around the same database handle share transactions, while distinct handles/pools
remain separate even if they connect to the same database server. This ID is
process-local, not a durable identifier. It uses `c.DB`, not `&c.DB`, so copying
the adapter does not change its identity.

## Active Transaction Detection

`Manager.InTx(ctx)` and `flsql.ConnectionAdapter.InTx(ctx)` implement the optional
`comproto.InTx` capability. They report whether the context carries an active
scope for that resource and transaction type. Plain contexts return false; live
contexts derived with `WithValue`, `WithCancel`, `WithDeadline`, or `WithoutCancel`
retain the scope.

A context with an error returns false. A completed current scope or transaction
ancestor also returns false, even through `context.WithoutCancel`. Detaching
cancellation alone does not complete a scope; detection tracks protocol-managed
completion, not native driver health. Committing an inner scope leaves its parent
active. Rolling back an inner scope may finish the parent as well.

`LookupTx` and `Q` retain their existing behaviour: they can still resolve the
native transaction after completion or cancellation, rather than silently falling
back to the resource. Calls to `InTx` must be serialized with `CommitTx` and
`RollbackTx` for the same transaction; this capability adds no concurrency guarantee.

## Transactional Resource

`Manager.R` is the underlying transactional resource. In the standard
database example it is a `*sql.DB`, but `flsql.ConnectionAdapter` also uses
`*pgxpool.Pool`. Other implementations can use any handle whose `Begin`
function returns a `*TX`. `flsql.ConnectionAdapter` deliberately keeps its
`DB` field name because it specifically holds a SQL database pool.

## Dynamic Rollback Mechanism

`txkit` dynamically adapts to refactored business logic, tying rollback steps to small atomic changes rather than high-level scenarios. This flexibility simplifies maintaining the rollback mechanism as your code evolves.
