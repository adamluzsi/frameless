package frameless_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queryusecases"
)

func ExampleQueryUseCase_storageFind(storage frameless.Storage) error {
	// InactiveUsers is a custom application specific query use case and specified by test next to the controller who use it.
	iterator := storage.Find(InactiveUsers{})

	for iterator.Next() {
		var user User

		if err := iterator.Decode(&user); err != nil {
			return err
		}

		// do something with inactive User
	}

	if err := iterator.Err(); err != nil {
		return err
	}

	return nil
}

func ExampleQueryUseCase_storageExec(storage frameless.Storage) error {
	// DeleteByID is a common query use case which specified with test in the queryusecases package
	// Of course you can implement your own as well
	return storage.Exec(queryusecases.DeleteByID{Type: User{}, ID: "42"})
}
