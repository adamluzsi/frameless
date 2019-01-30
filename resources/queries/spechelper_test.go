package queries_test

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

type testable func(t *testing.T, resource resources.Resource)

func (fn testable) Test(t *testing.T, resource resources.Resource) {
	fn(t, resource)
}

type IDInFieldName struct {
	ID string
}

type IDInTagName struct {
	DI string `ext:"ID"`
}

type IDInTagNameNextToIDField struct {
	ID string
	DI string `ext:"ID"`
}

type UnidentifiableID struct {
	UserID string
}

type InterfaceObject interface{}

type StructObject struct{}
