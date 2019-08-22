package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type CommonSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject interface {
		MinimumRequirements
		resources.Updater
	}
}

func (spec CommonSpec) Test(t *testing.T) {
	SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	UpdaterSpec{EntityType: spec.EntityType, FixtureFactory: spec.FixtureFactory, Subject: spec.Subject}.Test(t)
	DeleterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
}

func (spec CommonSpec) Benchmark(b *testing.B) {
	SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	UpdaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	DeleterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
}
