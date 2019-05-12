package storages

import (
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources/specs"
	"testing"
)

func TestCommonSpec(t *testing.T, r specs.Resource) {
	CommonSpec{Subject:r}.Test(t)
}

type CommonSpec struct {
	Subject        specs.Resource
	FixtureFactory GenericFixtureFactory
}

func (spec CommonSpec) Test(t *testing.T) {
	specs.TestAll(t, spec.Subject, ExportedEntity{}, spec.FixtureFactory)
	specs.TestAll(t, spec.Subject, unexportedEntity{}, spec.FixtureFactory)
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

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
