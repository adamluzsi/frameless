package crud

// TODO: mark ID as comparable

import (
	"context"

	"github.com/adamluzsi/frameless/ports/iterators"
)

type Creator[Entity any] interface {
	// Create takes a ptr to a entity<V> and store it into the resource.
	// It also updates the entity<V> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Create error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Create(ctx context.Context, ptr *Entity) error
}

type Finder[Entity, ID any] interface {
	ByIDFinder[Entity, ID]
	AllFinder[Entity]
}

type ByIDFinder[Entity, ID any] interface {
	// FindByID will link an entity that is found in the resource to the received ptr,
	// and report back if it succeeded finding the entity in the resource.
	// It also reports if there was an unexpected exception during the execution.
	// It was an intentional decision to not use error to represent "not found" case,
	// but tell explicitly this information in the form of return bool value.
	//
	//
	// Why the return signature includes a found bool value?
	//
	// It serves two crucial goals.
	// First, it forces the user through the go-vet tool that the unused variable check
	// will fail when the found bool variable is not checked before the entity is used.
	// It also improves readability and expresses the function's cyclomatic complexity.
	//   -> total: 2^(n+1+1)
	//     -> found/bool 2^(n+1)  | An entity might be found or not.
	//     -> error 2^(n+1)       | An error might occur or not.
	//
	// Last but not least, it eliminates the possibility that you return an initialized pointer type
	// that has no value and eventually causes a runtime error
	// if you provide that valid but nil pointer to an interface variable type.
	//   -> (MyInterface)((*Entity)(nil)) != nil
	//
	// You may find a similar approach in the standard library as `sql` null value types
	// and the environment lookup in the os package.
	FindByID(ctx context.Context, id ID) (ent Entity, found bool, err error)
}

type AllFinder[Entity any] interface {
	// FindAll will return all entity that has <V> type
	FindAll(context.Context) iterators.Iterator[Entity]
}

type Updater[Entity any] interface {
	// Update will takes a ptr that points to an entity
	// and update the corresponding stored entity with the received entity field values
	Update(ctx context.Context, ptr *Entity) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter[ID any] interface {
	ByIDDeleter[ID]
	AllDeleter
}

type ByIDDeleter[ID any] interface {
	// DeleteByID will remove a <V> type entity from the repository by a given ID
	DeleteByID(ctx context.Context, id ID) error
}

type AllDeleter interface {
	// DeleteAll will erase all entity from the resource that has <V> type
	DeleteAll(context.Context) error
}

// Purger supplies functionality to purge a resource completely.
// On high level this looks similar to what Deleter do,
// but in case of an event logged resource, this will purge all the events.
// After a purge, it is not expected to have anything in the repository.
// It is heavily discouraged to use Purge for domain interactions.
type Purger interface {
	// Purge will completely wipe all state from the given resource.
	// It is meant to be used in testing during clean-ahead arrangements.
	Purge(context.Context) error
}
