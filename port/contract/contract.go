package contract

import (
	"testing"

	"go.llib.dev/testcase"
)

// Init idiom provides a convenient way to define a configuration function within a contract.
// It returns a configuration result which will be used by the individual tests within a contract.
// As requirements arise, the configuration fields can be extended in a Open‑Closed‑Principle style.
//
// In testing scenarios, “Init” is expected to be used once.
// You can use this in scenarios where you need to manage state regarding your Config setup.
// For example, if you need to generate unique values as part of the configuration's make value function,
// simply create a local variable in the Init fucntion's scope and cache the already generated values there.
type Init[Config any] = func(testing.TB) Config

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
	testcase.OpenSuite
}
