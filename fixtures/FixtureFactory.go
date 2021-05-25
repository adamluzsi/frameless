package fixtures

import (
	"context"
	"reflect"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/frameless/reflects"
)

type FixtureFactory struct{}

func (f FixtureFactory) Create(T frameless.T) interface{} {
	v := reflect.New(reflects.BaseTypeOf(T))
	fv := reflects.BaseValueOf(New(T))

	field, _, ok := extid.LookupStructField(T)
	for i := 0; i < fv.NumField(); i++ {
		if ok && fv.Type().Field(i).Name == field.Name {
			continue
		}

		v.Elem().Field(i).Set(fv.Field(i))
	}

	return v.Interface()
}

func (f FixtureFactory) Context() (ctx context.Context) {
	return context.Background()
}
