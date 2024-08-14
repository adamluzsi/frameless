package contract

import (
	"go.llib.dev/testcase"
)

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
