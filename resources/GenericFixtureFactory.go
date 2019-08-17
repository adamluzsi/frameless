package resources

import (
	"context"
	"github.com/adamluzsi/frameless/fixtures"
)

type GenericFixtureFactory struct{}

func (f GenericFixtureFactory) Create(entity interface{}) interface{} {
	newEntity := fixtures.New(entity)
	if _, ok := LookupID(newEntity); ok {
		if err := SetID(newEntity, ""); err != nil {
			panic(err.Error())
		}
	}
	return newEntity
}

func (f GenericFixtureFactory) Context() (ctx context.Context) {
	return context.Background()
}