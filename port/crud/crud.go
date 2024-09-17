package crud

// TODO: mark ID as comparable

import (
	"context"

	"go.llib.dev/frameless/port/iterators"
)

type Creator[ENT any] interface {
	// Create is a function that takes a pointer to an entity and stores it in an external resource.
	// And external resource could be a backing service like PostgreSQL.
	// The use of a pointer type allows the function to update the entity's ID value,
	// which is significant in both the external resource and the domain layer.
	// The ID is essential because entities in the backing service are referenced using their IDs,
	// which is why the ID value is included as part of the entity structure fieldset.
	//
	// The pointer is also employed for other fields managed by the external resource, such as UpdatedAt, CreatedAt,
	// and any other fields present in the domain entity but controlled by the external resource.
	Create(ctx context.Context, ptr *ENT) error
}

type Finder[ENT, ID any] interface {
	ByIDFinder[ENT, ID]
	AllFinder[ENT]
}

type ByIDFinder[ENT, ID any] interface {
	// FindByID is a function that tries to find an ENT using its ID.
	// It will inform you if it successfully located the entity or if there was an unexpected issue during the process.
	// Instead of using an error to represent a "not found" situation,
	// a return boolean value is used to provide this information explicitly.
	//
	//
	// Why the return signature includes a found bool value?
	//
	// This approach serves two key purposes.
	// First, it ensures that the go-vet tool checks if the 'found' boolean variable is reviewed before using the entity.
	// Second, it enhances readability and demonstrates the function's cyclomatic complexity.
	//   total: 2^(n+1+1)
	//     -> found/bool 2^(n+1)  | An entity might be found or not.
	//     -> error 2^(n+1)       | An error might occur or not.
	//
	// Additionally, this method prevents returning an initialized pointer type with no value,
	// which could lead to a runtime error if a valid but nil pointer is given to an interface variable type.
	//   (MyInterface)((*ENT)(nil)) != nil
	//
	// Similar approaches can be found in the standard library,
	// such as SQL null value types and environment lookup in the os package.
	FindByID(ctx context.Context, id ID) (ent ENT, found bool, err error)
}

type ByIDsFinder[ENT, ID any] interface {
	// FindByIDs finds entities with the given IDs in the repository.
	// If any of the ID points to a non-existent ENT, the returned iterator will eventually yield an error.
	FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[ENT]
}

type AllFinder[ENT any] interface {
	// FindAll will return all entity that has <V> type
	// TODO: consider using error as 2nd argument, to make it similar to sql package
	FindAll(context.Context) iterators.Iterator[ENT]
}

type Updater[ENT any] interface {
	// Update will take a pointer to an entity and update the stored entity data by the values in received entity.
	// The ENT must have a valid ID field, which referencing an existing entity in the external resource.
	Update(ctx context.Context, ptr *ENT) error
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

type Saver[ENT any] interface {
	// Save combines the behaviour of Creator and Updater in a single functionality.
	// If the entity is absent in the resource, the entity is created based on the Creator's behaviour.
	// If the entity is present in the resource, the entity is updated based on the Updater's behaviour.
	// Save requires the entity to have a valid non-empty ID value.
	Save(ctx context.Context, ptr *ENT) error
}
