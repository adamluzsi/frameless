package contracts

import "testing"

// Interface represent a resource specification also known as "contract".
//
// The main goal of a resource spec is to introduce dependency injection pattern
// at logical level between consumers and suppliers.
// In other words any expectations from a consumer/interactor/use-case towards a used dependency
// should be defined in a contract.
// This allows architecture flexibility since the expectations not bound to a certain technology,
// but purely high level and as such can be implemented in various ways.
//
// Using resource spec also force the writer of the spec to keep things
// at high level and only focus on the expected behavior,
// instead of going into implementation details.
//
type Interface interface {
	Test(t *testing.T)
	Benchmark(b *testing.B)
}

func Run(tb testing.TB, specs ...Interface) {
	for _, spec := range specs {
		spec := spec
		switch tb := tb.(type) {
		case *testing.T:
			spec.Test(tb)
		case *testing.B:
			spec.Benchmark(tb)
		default:
			tb.Fatalf(`unknown test runner type: %T`, tb)
		}
	}
}
