package crud_test

import (
	"context"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper/testent"
)

var _ crud.LookupIDFunc[testent.Foo, testent.FooID] = extid.Lookup[testent.FooID, testent.Foo]

func ExampleFindMany() {

	type MyTypeID string

	type MyType struct {
		ID       MyTypeID
		Val      string
		IsActive bool
	}

	type MyTypeQuery interface {
		Fetch(ctx context.Context) (iterators.Iterator[MyType], error)
		Find(ctx context.Context) (MyType, bool, error)
		IDIs(MyTypeID) MyTypeQuery
		IsActiveIs(bool) MyTypeQuery
		ValIs(string) MyTypeQuery
	}

	type MyTypeRepository struct {
		crud.Creator[MyType]
		crud.FindOne[MyTypeQuery, MyType]
		crud.FindMany[MyTypeQuery, MyType]
	}

}
