package migration

import (
	"context"
	"sort"
)

type Migratable interface {
	Migrate(context.Context) error
}

// Group represents a coherent collection of migration steps associated with a specific topic or namespace.
// This struct serves as a means to encapsulate a set of migration steps
// that should be executed together or within the context of a designated topic.
type Group[R any] struct {
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
	Steps []Step[R]
}

// Step defines the behavior for an individual migration action.
//
// R is a type parameter signifying a shared resource used across multiple migration steps.
// For instance, R could represent a live database connection or a transactional context.
type Step[R any] interface {
	// MigrateUp applies the specific migration action associated with this step.
	// It is assumed that this step has not been executed prior to this invocation.
	//
	// Upon failure, it is up to the Migrator to undo the resources changes
	MigrateUp(R, context.Context) error
	// MigrateDown reverses the changes made by the corresponding MigrateUp action.
	// It is assumed that MigrateUp was successfully executed before calling this method.
	MigrateDown(R, context.Context) error
}

// Version of the version identifier of a migration step.
// UNIX timestamp, or an incrementaly increasing integer values are often used for this purpose.
// The only constraint for a Version number that it must be sortable by string character comparison.
type Version string

// Steps is a group of Migration Steps
type Steps[R any] map[Version]Step[R]

type stage[R any] struct {
	Version Version
	Step    Step[R]
}

func (s Steps[R]) stages() []stage[R] {
	var stages []stage[R]
	for version, step := range s {
		stages = append(stages, stage[R]{
			Version: version,
			Step:    step,
		})
	}
	sort.Slice(stages, func(i, j int) bool {
		return stages[i].Version < stages[j].Version
	})
	return stages
}
