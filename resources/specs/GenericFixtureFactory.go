package specs

import (
	"context"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
)

type GenericFixtureFactory struct{}

func (f GenericFixtureFactory) Create(entity interface{}) interface{} {
	newEntity := fixtures.New(entity)
	if _, ok := resources.LookupID(newEntity); ok {
		if err := resources.SetID(newEntity, ""); err != nil {
			panic(err.Error())
		}
	}
	return newEntity
}

func (f GenericFixtureFactory) Context() (ctx context.Context) {
	return context.Background()
}
