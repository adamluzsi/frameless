package frameless

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/reflects"
	"io"
	"reflect"

	"github.com/adamluzsi/frameless/iterators"
)

//------------------------------------------------------ C/R/U/D -----------------------------------------------------//

type Creator interface {
	// Create takes a ptr to a entity<T> and store it into the resource.
	// It also updates the entity<T> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Create error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Create(ctx context.Context, ptr interface{}) error
}

type Finder interface {
	// FindByID will link an entity that is found in the resource to the received ptr,
	// and report back if it succeeded finding the entity in the resource.
	// It also reports if there was an unexpected exception during the execution.
	// It was an intentional decision to not use error to represent "not found" case,
	// but tell explicitly this information in the form of return bool value.
	FindByID(ctx context.Context, ptr, id interface{}) (found bool, err error)
	// FindAll will return all entity that has <T> type
	FindAll(context.Context, T) iterators.Interface
}

type Updater interface {
	// Update will takes a ptr that points to an entity
	// and update the corresponding stored entity with the received entity field values
	Update(ctx context.Context, ptr interface{}) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter interface {
	// DeleteByID will remove a <T> type entity from the storage by a given ID
	DeleteByID(ctx context.Context, T T, id interface{}) error
	// DeleteAll will erase all entity from the resource that has <T> type
	DeleteAll(context.Context, T) error
}

//------------------------------------------------------ Pub/Sub -----------------------------------------------------//

type CreatorPublisher interface {
	// SubscribeToCreate create a subscription to create event feed.
	// 	eg.: storage.SubscribeToCreate(``, cache.CreateEventHandler())
	//
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToCreate(context.Context, T, Subscriber) (Subscription, error)
}

type UpdaterPublisher interface {
	// SubscribeToUpdate create a subscription to the update event feed.
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToUpdate(context.Context, T, Subscriber) (Subscription, error)
}

type DeleterPublisher interface {
	SubscribeToDeleteByID(context.Context, T, Subscriber) (Subscription, error)
	SubscribeToDeleteAll(context.Context, T, Subscriber) (Subscription, error)
}

type Subscription interface {
	io.Closer
}

type Subscriber interface {
	// Handle handles the the subscribed event.
	// Context may or may not have meta information about the received event.
	// To ensure expectations, define a resource specification <contract> about what must be included in the context.
	Handle(ctx context.Context, ent interface{}) error
	// Error allow the subscription implementation to be notified about unexpected situations
	// that needs to be handled by the subscriber.
	// For e.g. the connection is lost and the subscriber might have cached values
	// that must be invalidated on the next successful Handle call
	Error(ctx context.Context, err error) error
}

//--------------------------------------------------------------------------------------------------------------------//

func SetID(ptr interface{}, id interface{}) error {
	r := reflect.ValueOf(ptr)

	if r.Kind() != reflect.Ptr {
		return errors.New("ptr should be given, else Pass By Value prevent setting struct ID field remotely")
	}

	_, val, ok := LookupIDStructField(ptr)

	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func LookupID(i interface{}) (id interface{}, ok bool) {
	_, val, ok := LookupIDStructField(i)

	if !ok {
		return nil, false
	}

	return val.Interface(), !isNil(val)
}

func isNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		return isNil(val.Elem())

	case reflect.Ptr, reflect.Slice, reflect.Chan, reflect.Func, reflect.Map:
		return val.IsNil()

	default:
		return !val.IsValid() || val.IsZero()

	}
}

func LookupIDStructField(ent interface{}) (reflect.StructField, reflect.Value, bool) {
	val := reflects.BaseValueOf(ent)

	sf, byTag, ok := lookupByTag(val)
	if ok {
		return sf, byTag, true
	}

	const name = `ID`
	if byName := val.FieldByName(name); byName.Kind() != reflect.Invalid {
		sf, _ := val.Type().FieldByName(name)
		return sf, byName, true
	}

	return reflect.StructField{}, reflect.Value{}, false

}

func lookupByTag(val reflect.Value) (reflect.StructField, reflect.Value, bool) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		structField := val.Type().Field(i)
		tag := structField.Tag

		if tag.Get("ext") == "ID" {
			return structField, valueField, true
		}
	}

	return reflect.StructField{}, reflect.Value{}, false

}

//--------------------------------------------------------------------------------------------------------------------//
