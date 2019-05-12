package resources

import (
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources/specs"
)

type GenericFixtureFactory struct{}

func (f GenericFixtureFactory) Create(entity interface{}) interface{} {
	newEntity := fixtures.New(entity)
	if _, ok := specs.LookupID(newEntity); ok {
		if err := specs.SetID(newEntity, ""); err != nil {
			panic(err.Error())
		}
	}
	return newEntity
}
