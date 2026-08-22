package workflow

import (
	"context"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

type ForEach struct {
	Collection VarName
	// Key [optional] will be used to store either the index value of a slice, or the key of a map in the process variables
	Key VarName
	// Value is where the value/element will be placed in the process variables
	Value VarName
}

var _ Definition = ForEach{}

func (foreach ForEach) Execute(ctx context.Context, processID ProcessID) error {
	vars, err := getVarsFor(ctx, processID)
	if err != nil {
		return err
	}

	coll, ok, err := vars.Lookup(ctx, foreach.Collection)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	collection := reflect.ValueOf(coll)
	collection = reflectkit.BaseValue(collection)

	switch collection.Kind() {
	case reflect.Slice:
		reflectkit.IterSlice(collection)
	default:
		return nil
	}
	return nil
}

func (foreach ForEach) Error() string { return "workflow::foreach" }
