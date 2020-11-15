package fixtures

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"reflect"

	"github.com/adamluzsi/frameless/resources"
)

type FixtureFactory struct{}

func (f FixtureFactory) Create(T resources.T) interface{} {
	v := reflect.New(reflects.BaseTypeOf(T))
	fv := reflects.BaseValueOf(New(T))

	field, _, ok := resources.LookupIDStructField(T)
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
