package contract

import (
	"testing"

	"go.llib.dev/testcase"
)

// Make func meant to create a new instance of the testing subject.
// In testing suites, we often focus on the type of interface that is being examined.
//
// When a test requires multiple specifications—so simple option idioms aren't sufficient,
// we can create a "XXXSubject" struct that contains all the necessary dependencies as fields.
// This approach lets us use a single ‘Make" function to set up each testing case within a contract,
// while keeping the configuration extensible in an open‑closed‑principle style.
//
// The "Make" function should be called only once and must provide a field capable of creating the interface value.
//
// Feel free to explore this pattern for your testing needs.
type Make[Subject any] = func(tb testing.TB) Subject

// Contract represents a resource specification also known as "contract".
//
// The main goal of a contract is to introduce dependency injection pattern at logical level between a consumer its supplier.
//
// In other words any expectations from a consumer/interactor/use-case towards a used dependency
// should be defined in a contract.
// This allows architecture flexibility since the expectations not bound to a certain technology,
// but purely high level and as such can be implemented in various ways.
//
// Using resource Spec also force the writer of the Spec to keep things
// at high level and only focus on the expected behavior,
// instead of going into implementation details.
type Contract interface {
	testcase.Suite
	// Test is the function that assert expected behavioral requirements from a supplier implementation.
	// These behavioral assumptions made by the Consumer in order to simplify and stabilise its own code complexity.
	// Every time a Consumer makes an assumption about the behavior of the role interface supplier,
	// it should be clearly defined it with tests under this functionality.
	Test(*testing.T)
	// Benchmark will help with what to measure.
	// When you define a role interface contract, you should clearly know what performance aspects important for your Consumer.
	// Those aspects should be expressed in a form of Benchmark,
	// so different supplier implementations can be easily A/B tested from this aspect as well.
	Benchmark(*testing.B)
}
