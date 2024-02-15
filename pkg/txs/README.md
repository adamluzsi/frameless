# `txs` Package

The `txs` package in Go simplifies transaction management within an use-case code level.

The `txs` package serves a crucial role in managing transactions, especially in environments lacking native support.
It proves invaluable when dealing with stateful systems, such as APIs without built-in transactional capabilities.
The package ensures that if an error occurs during an operation,
all stateful changes are gracefully undone in a Last-In-First-Out (LIFO) manner, akin to how `defer` operates.

While it doesn't replace proper transaction support in a mathematical sense,
it adeptly handles about 99% of scenarios where manual intervention was historically required.
This significantly reduces the occasions where human interaction is necessary, making it almost negligible.

It's important to note that the `txs` package can seamlessly integrate with real transactions if available,
but its true strength lies in mitigating challenges when such support is absent.

```go
package mypkg

import (
	"context"
	"go.llib.dev/frameless/pkg/txs"
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
		return RollbackForMyActionWhichMutatedTheSystemState(ctx)
	})

	return nil
}

```

## Challenges without `txs`

In systems without transactional support, managing the complexity of rollback logic increases
with the number of actions that create entities in external resources.
For instance, when creating entities `A`, `B`, `C`, and `D`,
encountering an error during the creation of `C` requires undoing `A` and `B`.
If an error occurs during the creation of D, the rollback should include `A`, `B`, and `C`.
As actions become more intricate, maintaining a comprehensive rollback mechanism becomes challenging.

## How `txs` Simplifies

The `txs` package simplifies this complexity by allowing the registration of an OnRollback callback after each
successful action that mutates the system state. When an error occurs, rolling back the current `txs` transaction
executes all prepared rollback steps in a Last-In-First-Out (LIFO) order, akin to defer functionality.
This streamlined approach ensures the restoration of a stable system state.

## Reduced the required Mental Model Size

With `txs`, your code focuses on individual entities and associated actions,
reducing the mental model needed for rollback.
For example, the code for creating entity `C` involves initiating a transaction, performing the action (creating
C), defining rollback steps (deleting C),
and finishing the transaction with either Commit or Rollback in case of an error.

## Nested Transactions

The package seamlessly supports nested transactions, automatically joining an existing transaction if present in the
context. This facilitates a hierarchical structure
where smaller transactions combine to form larger units without the need for code updates.

## Dynamic Rollback Mechanism

`txs` dynamically adapts to refactored business logic, tying rollback steps to small atomic changes rather than
high-level scenarios. This flexibility simplifies rollback mechanism maintenance as your code evolves.
