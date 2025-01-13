package reflectkit_test

import (
	"fmt"
	"math/rand"

	"go.llib.dev/frameless/pkg/reflectkit"
)

func ExampleFullyQualifiedName() {
	fmt.Println(reflectkit.FullyQualifiedName(Example{}))
}

func ExampleSymbolicName() {
	fmt.Println(reflectkit.SymbolicName(Example{}))
}

func ExampleLink() {
	var src = Example{Name: "Panda"}
	var dest Example

	if err := reflectkit.Link(src, &dest); err != nil {
		// handle err
	}
}

type Example struct {
	Name string
}

type IDInFieldName struct {
	ID string
}

type IDInTagName struct {
	DI string `ext:"ID"`
}

type UnidentifiableID struct {
	UserID string
}

type InterfaceObject interface{ Method() }

type StructObject struct{ V string }

func (StructObject) Method() {}

var RandomName = fmt.Sprintf("%d", rand.Int())
