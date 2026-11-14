package crudcontract

import (
	"go.llib.dev/testcase"
)

// raceConcurrently runs the supplied closures concurrently via testcase.Race.
//
// The closures are expected to be already prepared with the testcase variables
// they need (e.g. via `testcase.Let#get` at the spec body, not inside the
// closure itself). Calling `Var.Get` from inside a closure that runs under
// testcase.Race is unsafe because testcase's lazy variable initialisation is
// not designed to fire from multiple goroutines at once.
//
// Run with `-race` to catch regressions: any data race detected inside the
// closures is a contract violation by the implementation under test.
func raceConcurrently(ops []func()) {
	if len(ops) == 0 {
		return
	}
	testcase.Race(ops...)
}

// makeConcurrentCalls returns n no-op races of a single closure template.
// It is a thin convenience for the common pattern in the contracts:
//
//	t.Random.Repeat(2, 4, func() {
//	    ops = append(ops, func() { subject.X(...) })
//	})
//	testcase.Race(ops...)
//
// n is chosen as a small range so the test runs quickly while still giving
// the race detector several goroutines to inspect.
func makeConcurrentCalls(n int, mk func(i int) func()) []func() {
	if n <= 0 {
		return nil
	}
	ops := make([]func(), 0, n)
	for i := 0; i < n; i++ {
		ops = append(ops, mk(i))
	}
	return ops
}
