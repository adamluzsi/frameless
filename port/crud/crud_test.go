package crud_test

import (
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/spechelper/testent"
)

var _ crud.LookupIDFunc[testent.Foo, testent.FooID] = extid.Lookup[testent.FooID, testent.Foo]
