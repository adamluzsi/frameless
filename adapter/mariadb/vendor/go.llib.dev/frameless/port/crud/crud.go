package crud

// TODO: mark ID as comparable

import (
	"context"
	"io"
	"iter"
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
	FindByIDs(ctx context.Context, ids ...ID) iter.Seq2[ENT, error]
}

type AllFinder[ENT any] interface {
	// FindAll will return all entity that has <V> type
	// TODO: consider using error as 2nd argument, to make it similar to sql package
	FindAll(context.Context) iter.Seq2[ENT, error]
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
// On high level this looks similar to what AllDeleter do,
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

type Batcher[ENT any, BATCH Batch[ENT]] interface {
	// Batch lets you add multiple entities to a resource in one go, using a stream processing approach.
	//
	// Unlike Creator[ENT], Batch doesn't assign or update fields like id.
	// If you need IDs to be generated automatically, please use Creator[ENT],
	// or provide the IDs ahead of time when using Batch.
	Batch(ctx context.Context) BATCH
}

type Batch[ENT any] interface {
	// Add will prepare an entity for batch insertion.
	// It is not guaranteed that at the time of adding, the entity is created in the resource.
	Add(ENT) error
	// Closer signals the end of the batch addition process.
	// When called, it ensures any pending operations to add entities to a resource are completed smoothly.
	// Cancellation should be handled through the context you provided at the start.
	// Ideally, nn error received during Close should represent an atomic failure for the whole batch.
	//
	// Calling Close is mandatory, and should be at least deferred.
	// Calling Close multiple times should be idempotent.
	io.Closer
}

// QueryOneMethodSignature defines the structure of a "query one" method signature.
// It outlines how the method should retrieve a single entity and communicate the outcome.
//
// The method returns three values:
// - The requested entity (_ENT)
// - A boolean 'found' indicating if the entity was located
// - An error if something went wrong during execution
//
// Instead of using an error to signal a "not found" case, this signature uses the boolean 'found'.
// This clearly separates cases where the entity was not found from actual errors,
// making it explicit that no error occurred, but simply no matching entity was located.
//
// Why return a boolean 'found' instead of using a nil pointer *ENT value?
//
// There are several reasons for this:
// - The go-vet tool ensures that the 'found' variable is checked before using the entity.
// - It improves readability and highlights the method's cyclomatic complexity by making the flow easier to understand.
//
// The method signature express the following cyclomatic complexity factors:
// - found/bool: Entity found (true) or not (false)
// - error: An error occurred (true) or did not (false)
//
// This approach also prevents returning an initialized pointer that may be nil,
// which could lead to a runtime error when cast to an interface:
//
//	(MyInterface)((*ENT)(nil)) != nil
//
// Similar patterns exist in the standard library,
// such as handling SQL null values and environment variable lookups in the os package.
type QueryOneMethodSignature[ENT, ARGS any] func(context.Context, ARGS) (_ ENT, found bool, _ error)

// QueryManyMethodSignature defines the structure of a "query many" method signature,
// designed to handle queries that return an unknown number of entities.
//
// The method returns two values:
// - An iterator of entities (iter.Seq[ENT]), allowing efficient retrieval of multiple results
// - An error, which is only returned if there was a fundamental issue with the query itself
//
// The use of an iterator is key when the result set size is unknown or large.
// It enables processing the results as they are fetched, without needing to load everything into memory at once.
//
// This pattern also improves resource management by fetching results incrementally,
// making it suitable for working with large datasets or slow data sources.
//
// Why return an iterator?
//
//  1. It provides flexibility in handling a varying number of entities, allowing the caller
//     to iterate over results efficiently, without requiring all data upfront.
//  2. It separates concerns, using an error return to indicate issues with the query execution.
//  3. It enables the support for supporting a streaming gateway to our application.
//
// Similar to standard library patterns like SQL row iterators,
// this approach offers control over how the caller consumes the results,
// ensuring both performance and clarity in handling multiple entities.
type QueryManyMethodSignature[ENT, ARGS any] func(context.Context, ARGS) iter.Seq2[ENT, error]
