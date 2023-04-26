# Package `txs`

The `txs` package is a Go package designed to manage transactions in the context of an application.
Its primary purpose is to provide a mechanism to handle transaction management,
with functionalities such as beginning, committing, rolling back, and adding rollback steps.

This is particularly useful when working with systems that don't inherently support transaction management.
In situations where undoing previous actions is essential for maintaining a stable system state
when an error occurs midway through our actions, the `txs` package can provide valuable support.

Traditionally, as the number of actions that create entities in external resources increases, 
the complexity of rollback logic also increases without the support of a transaction framework. 

For example, if an action involves creating `A`, `B`, `C`,
and `D`, and an error occurs during the creation of `C`,
we need to undo `A` and `B`.
However, if an error occurs during the creation of `D`, 
we need to undo `A`, `B`, and `C`.
The more complex our actions gets,
the more difficult it is to maintain this complex rollback logic,
and the more challenging it becomes to apply changes on the system
without missing something from our the rollback mechanism. 

The `txs` package simplifies this process by allowing you to register an OnRollback callback
on a given context after each successful action that mutates a system state.
This means that when something goes wrong, you just need to rollback the current `txs` transaction
which executes all the prepared rollback steps in a LIFO order similarly to how defer works,
making your application restore a stable system state again.

This approach reduce the needed mental model about what needs to be rolled back.
With the previous example, your code focus on just one entity, let's say C.
Your code essentially focus on these steps:
- begin a tx
- do your action which mutates the system state
  - create C
- after success, define what to do when a rollback is needed
  - arrange on rollback, do delete C 
- finish your transaction with either Commit 
  or in case of an error then with a Rollback

This works at a small scale, and enables a small mental model,
and if the context already contained a transaction,
then our new transaction joins in and becomes a nested transaction.
When transactions nest and combine, 
they forming a larger unit without the need to update the code to be aware of this.
So if a higher level fails, because creating `D` had an issue, `C` rollback is executed. 

The rollback mechanism dynamically follows refactored business logic,
as it is tied to small atomic changes rather than high-level scenarios
freeing you complexity in the rollback mechanism maintenance. 

```go
package mypkg

// ...

func MyUseCase(ctx context.Context) (returnErr error) {
	// automatically joins if there is a transaction already in the received context
	ctx, err := txs.Begin(ctx)
	if err != nil {
		return err
	}
	defer txs.Finish(&returnErr, ctx)

	if err := MyActionWhichMutateTheSystemState(ctx); err != nil {
		return err
	}

	txs.OnRollback(ctx, func(ctx context.Context) error {
		return RollbackForMyActionWhichMutatedTheSystemState(ctx)
	})

	return nil
}

```