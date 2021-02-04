package contracts

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
)

type CRUD struct {
	Subject interface {
		minimumRequirements
		resources.Updater
	}
	T interface{}
	FixtureFactory
}

func (spec CRUD) Test(t *testing.T) {
	t.Run(reflects.SymbolicName(spec.T), func(t *testing.T) {
		Creator{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Finder{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Updater{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		Deleter{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})
}

func (spec CRUD) Benchmark(b *testing.B) {
	b.Run(reflects.SymbolicName(spec.T), func(b *testing.B) {
		Creator{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Finder{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Updater{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		Deleter{T: spec.T, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})
}
