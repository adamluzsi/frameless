package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
)

type CRUD struct {
	EntityType interface{}
	FixtureFactory
	Subject interface {
		minimumRequirements
		resources.Updater
	}
}

func (spec CRUD) Test(t *testing.T) {
	t.Run(reflects.SymbolicName(spec.EntityType), func(t *testing.T) {
		Creator{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Finder{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Updater{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Deleter{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})
}

func (spec CRUD) Benchmark(b *testing.B) {
	b.Run(reflects.SymbolicName(spec.EntityType), func(b *testing.B) {
		Creator{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Finder{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Updater{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Deleter{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})
}
