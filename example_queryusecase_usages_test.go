package frameless_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries"
)

func ExampleQueryUseCase_storageFind(storage frameless.ExternalResource) error {
	// InactiveUsers is a custom application specific query use case and specified by test next to the controller who use it.
	iterator := storage.Exec(InactiveUsers{})

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

func ExampleQueryUseCase_storageExec(storage frameless.ExternalResource) error {
	// DeleteByID is a common query use case which specified with test in the queries package
	// Of course you can implement your own as well
	return storage.Exec(queries.DeleteByID{Type: User{}, ID: "42"}).Err()
}
