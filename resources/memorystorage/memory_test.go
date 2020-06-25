package memorystorage_test

import (
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources/memorystorage"
	"github.com/adamluzsi/frameless/resources/specs"

	"testing"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(t *testing.T) {
	subject := ExampleMemory()
	specs.CreatorSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.FinderSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.UpdaterSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.DeleterSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
	specs.OnePhaseCommitProtocolSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Test(t)
}

func BenchmarkMemory(b *testing.B) {
	subject := ExampleMemory()
	specs.CreatorSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.FinderSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.UpdaterSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.DeleterSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
	specs.OnePhaseCommitProtocolSpec{EntityType: exampleEntity{}, Subject: subject, FixtureFactory: fixtures.FixtureFactory{}}.Benchmark(b)
}

type exampleEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
