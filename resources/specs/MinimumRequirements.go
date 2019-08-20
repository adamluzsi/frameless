package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type MinimumRequirements interface {
	resources.Saver
	resources.Finder
	resources.Deleter
	resources.Truncater
}

type MinimumRequirementsSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec MinimumRequirementsSpec) Test(t *testing.T) {
	t.Run(`MinimumRequirementsSpec`, func(t *testing.T) {
		SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		DeleterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
		TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})
}

func (spec MinimumRequirementsSpec) Benchmark(b *testing.B) {
	b.Run(`MinimumRequirementsSpec`, func(b *testing.B) {
		SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		DeleterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})
}
