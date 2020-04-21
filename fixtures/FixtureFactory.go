package fixtures

import (
	"context"

	"github.com/adamluzsi/frameless/resources"
)

type FixtureFactory struct{}

func (f FixtureFactory) Create(entity interface{}) interface{} {
	newEntity := New(entity)
	if _, ok := resources.LookupID(newEntity); ok {
		if err := resources.SetID(newEntity, ""); err != nil {
			panic(err.Error())
		}
	}
	return newEntity
}

func (f FixtureFactory) Context() (ctx context.Context) {
	return context.Background()
}
