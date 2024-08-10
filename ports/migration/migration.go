package migration

import "context"

type Migratable interface {
	Migrate(context.Context) error
}

// Group represents a coherent collection of migration steps associated with a specific topic or namespace.
// This struct serves as a means to encapsulate a set of migration steps
// that should be executed together or within the context of a designated topic.
type Group[Resource any] struct {
	// ID field within the Group specifically denotes the intended domain or area, such as "foo-queue",
	// ensuring that migration steps within this group are uniquely identified and do not overlap
	// or interfere with migrations from other namespaces.
	ID string
	// Steps field is an ordered list of individual migration actions,
	// each capable of migrating up or down (applying or rolling back changes).
	// The generic type parameter `R` allows the migration steps to work with a shared resource,
	// which could be something like a live database connection or a transaction object.
	// This design provides flexibility and consistency,
	// ensuring that all steps in a given migration group have access to necessary shared resources.
	Steps []Step[Resource]
}

// Step defines the behavior for an individual migration action.
//
// Resource is a type parameter signifying a shared resource used across multiple migration steps.
// For instance, Resource could represent a live transaction.
// A migration Step is expected to operate on the Resource specifically, and has no state on its own.
type Step[Resource any] interface {
	UpMigration[Resource]
	DownMigration[Resource]
}

type UpMigration[Resource any] interface {
	// MigrateUp applies the specific migration action associated with this step.
	// It is assumed that this step has not been executed prior to this invocation.
	//
	// Upon failure, it is up to the Migrator to undo the resources changes
	MigrateUp(context.Context, Resource) error
}

type DownMigration[Resource any] interface {
	// MigrateDown reverses the changes made by the corresponding MigrateUp action.
	// It is assumed that MigrateUp was successfully executed before calling this method.
	MigrateDown(context.Context, Resource) error
}
