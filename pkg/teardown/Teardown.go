package teardown

import (
	"sync"

	"github.com/adamluzsi/testcase/sandbox"
)

type Teardown struct {
	mutex sync.Mutex
	fns   []func()
}

// Defer function defers the execution of a function until the current test case returns.
// Deferred functions are guaranteed to run, regardless of panics during the test case execution.
// Deferred function calls are pushed onto a testcase runtime stack.
// When an function passed to the Defer function, it will be executed as a deferred call in last-in-first-orderingOutput order.
//
// It is advised to use this inside a testcase.Spec#Let memorization function
// when spec variable defined that has finalizer requirements.
// This allow the specification to ensure the object finalizer requirements to be met,
// without using an testcase.Spec#After where the memorized function would be executed always, regardless of its actual need.
//
// In a practical example, this means that if you have common vars defined with testcase.Spec#Let memorization,
// which needs to be Closed for example, after the test case already run.
// Ensuring such objects Close call in an after block would cause an initialization of the memorized object list the time,
// even in tests where this is not needed.
//
// e.g.:
//	- mock initialization with mock controller, where the mock controller #Finish function must be executed after each testCase suite.
//	- sql.DB / sql.Tx
//	- basically anything that has the io.Closer interface
//
// https://github.com/golang/go/issues/41891
func (td *Teardown) Defer(fn func()) {
	td.mutex.Lock()
	defer td.mutex.Unlock()
	td.fns = append(td.fns, func() { td.withRecover(fn) })
}

func (td *Teardown) Finish() {
	// handle Deferred functions which are deferred during the execution of a defer function
	for !td.isEmpty() {
		td.run()
	}
}

func (td *Teardown) isEmpty() bool {
	return len(td.fns) == 0
}

func (td *Teardown) add(fn func()) {
	td.mutex.Lock()
	defer td.mutex.Unlock()
	td.fns = append(td.fns, func() { td.withRecover(fn) })
}

func (td *Teardown) withRecover(fn func()) {
	runOutcome := sandbox.Run(fn)
	if runOutcome.Goexit { // ignore goexit
		return
	}
	if !runOutcome.OK { // propagate panic
		panic(runOutcome.PanicValue)
	}
	return
}

func (td *Teardown) run() {
	td.mutex.Lock()
	fns := td.fns
	td.fns = nil
	td.mutex.Unlock()
	for _, cu := range fns {
		defer cu()
	}
}
