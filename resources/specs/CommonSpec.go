package specs

import (
	"testing"
)

type CommonSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject resource
}

func (spec CommonSpec) Test(t *testing.T) {
	t.Run(`CREATE`, func(t *testing.T) {
		SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})

	t.Run(`READ`, func(t *testing.T) {
		FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})

	t.Run(`UPDATE`, func(t *testing.T) {
		UpdaterSpec{EntityType: spec.EntityType, FixtureFactory: spec.FixtureFactory, Subject: spec.Subject}.Test(t)
	})

	t.Run(`DELETE`, func(t *testing.T) {
		DeleterSpec{Subject: spec.Subject, EntityType: spec.EntityType, FixtureFactory: spec.FixtureFactory}.Test(t)
		TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Test(t)
	})
}

func (spec CommonSpec) Benchmark(b *testing.B) {
	b.Run(`CREATE`, func(b *testing.B) {
		SaverSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})

	b.Run(`READ`, func(b *testing.B) {
		FinderSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})

	b.Run(`UPDATE`, func(b *testing.B) {
		UpdaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})

	b.Run(`DELETE`, func(b *testing.B) {
		DeleterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
		TruncaterSpec{EntityType: spec.EntityType, Subject: spec.Subject, FixtureFactory: spec.FixtureFactory}.Benchmark(b)
	})
}
