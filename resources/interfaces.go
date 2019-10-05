package resources

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type Saver interface {
	// Save takes a ptr to a entity<T> and save it into the resource.
	// It also updates the entity<T> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Save error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Save(ctx context.Context, ptr interface{}) error
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

type Truncater interface {
	// Truncate will erase all entity from the resource that has <T> type
	Truncate(ctx context.Context, T interface{}) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter interface {
	// DeleteByID will remove a <T> type entity from the storage by a given ID
	DeleteByID(ctx context.Context, T interface{}, id string) error
}
