package frameless

import (
	"context"
)

type Creator /* T */ interface {
	// Create takes a ptr to a entity<T> and store it into the resource.
	// It also updates the entity<T> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Create error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Create(ctx context.Context, ptr /* *T */ interface{}, opts ...interface{}) error
}

type Finder /* T, ID */ interface {
	// FindByID will link an entity that is found in the resource to the received ptr,
	// and report back if it succeeded finding the entity in the resource.
	// It also reports if there was an unexpected exception during the execution.
	// It was an intentional decision to not use error to represent "not found" case,
	// but tell explicitly this information in the form of return bool value.
	FindByID(ctx context.Context, ptr /* *T */, id /* ID */ ID) (found bool, err error)
	// FindAll will return all entity that has <T> type
	FindAll(context.Context) Iterator
}

type Updater /* T */ interface {
	// Update will takes a ptr that points to an entity
	// and update the corresponding stored entity with the received entity field values
	Update(ctx context.Context, ptr /* *T */ interface{}) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter /* ID */ interface {
	// DeleteByID will remove a <T> type entity from the storage by a given ID
	DeleteByID(ctx context.Context, id ID) error
	// DeleteAll will erase all entity from the resource that has <T> type
	DeleteAll(context.Context) error
}

// Purger supplies functionality to purge a resource completely.
// On high level this looks similar to what Deleter do,
// but in case of an event logged resource, this will purge all the events.
// After a purge, it is not expected to have anything in the storage.
// It is heavily discouraged to use Purge for domain interactions.
type Purger interface {
	// Purge will completely wipe all state from the given resource.
	// It is meant to be used in testing during clean-ahead arrangements.
	Purge(context.Context) error
}
