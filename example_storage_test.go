package frameless_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless/queryusecases"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
)

//
// mystorage.go

type MyStorage struct {
	ExternalResourceConnection interface{}
}

func (storage *MyStorage) Close() error {
	closer, ok := storage.ExternalResourceConnection.(io.Closer)

	if !ok {
		return nil
	}

	return closer.Close()
}

func (storage *MyStorage) Create(e frameless.Entity) error {
	switch e.(type) {
	case *MyEntity:
		myEntity := e.(*MyEntity)
		fmt.Println("persist in db", myEntity)
		return reflects.SetID(myEntity, "42")

	default:
		panic("not implemented")

	}
}

func (storage *MyStorage) Find(quc frameless.QueryUseCase) frameless.Iterator {
	switch quc.(type) {
	case queryusecases.ByID:
		// implementation for queryusecases.ByID with the given external resource connection
		ByID := quc.(queryusecases.ByID)

		fmt.Printf("searching in %s table for %s ID\n", reflects.Name(ByID.Type), ByID.ID)

		return iterators.NewEmpty()

	default:
		panic("not implemented")

	}
}

func (storage *MyStorage) Exec(quc frameless.QueryUseCase) error {
	switch quc.(type) {
	case queryusecases.DeleteByEntity:
		DeleteByEntity := quc.(queryusecases.DeleteByEntity)

		ID, found := reflects.LookupID(DeleteByEntity.Entity)

		if !found {
			return errors.New("this implementation depending on an ID field in the entity")
		}

		name := reflects.Name(DeleteByEntity.Entity)

		fmt.Printf("deleting from %s the row with the %s ID of\n", name, ID)

		return nil

	default:
		panic("not implemented")

	}
}

//
// mycustomstorage_test.go

func ThisIsHowYouCanCreateTestToTestQueryUseCaseIntegrationsIntoTheStorage(suite *testing.T) {
	suite.Run("QueryUseCase", func(spec *testing.T) {
		storage := &MyStorage{ExternalResourceConnection: nil}
		// or you can create NewMyStorage(interface{}) as well for controlled initialization of your storage implementation,
		// and use it here for initialize the object

		spec.Run("queryusecases.ByID", func(t *testing.T) {

			// this will test our implementation against the expected behavior in the ByID specification
			queryusecases.ByID{
				Type: MyEntity{},
				NewEntityForTest: func(interface{}) interface{} {
					return &MyEntity{}
				},
			}.Test(t, storage)
		})

	})
}

func ExampleStorage() {
	// for working implementation example check frameless/storages package in Memory storage code and test
}
