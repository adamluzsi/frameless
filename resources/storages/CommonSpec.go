package storages

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/specs"
)

type Resource interface {
	resources.Saver
	resources.Finder
	resources.Updater
	resources.Deleter
	resources.Truncater
}

type CommonSpec struct {
	Subject        Resource
	FixtureFactory specs.GenericFixtureFactory
}

func (spec CommonSpec) Test(t *testing.T) {
	specs.CommonSpec{
		Subject:        spec.Subject,
		EntityType:     ExportedEntity{},
		FixtureFactory: spec.FixtureFactory,
	}.Test(t)

	specs.CommonSpec{
		Subject:        spec.Subject,
		EntityType:     unexportedEntity{},
		FixtureFactory: spec.FixtureFactory,
	}.Test(t)
}

func (spec CommonSpec) Benchmark(b *testing.B) {
	specs.CommonSpec{
		Subject:        spec.Subject,
		EntityType:     ExportedEntity{},
		FixtureFactory: spec.FixtureFactory,
	}.Benchmark(b)
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
