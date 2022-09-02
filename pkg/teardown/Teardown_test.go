package teardown_test

import (
	"github.com/adamluzsi/frameless/pkg/teardown"
	"github.com/adamluzsi/testcase/sandbox"
	"runtime"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func TestTeardown_Defer_order(t *testing.T) {
	td := &teardown.Teardown{}
	var res []int
	td.Defer(func() { res = append(res, 3) })
	td.Defer(func() { res = append(res, 2) })
	td.Defer(func() { res = append(res, 1) })
	td.Defer(func() { res = append(res, 0) })
	td.Finish()
	//
	assert.Must(t).Equal([]int{0, 1, 2, 3}, res)
}

func TestTeardown_Defer_commonFunctionSignatures(t *testing.T) {
	td := &teardown.Teardown{}
	var res []int
	td.Defer(func() { res = append(res, 1) })
	td.Defer(func() { res = append(res, 0) })
	td.Finish()
	//
	assert.Must(t).Equal([]int{0, 1}, res)
}

func TestTeardown_Defer_ignoresGoExit(t *testing.T) {
	t.Run(`spike`, func(t *testing.T) {
		var a, b, c bool
		out := sandbox.Run(func() {
			td := &teardown.Teardown{}
			td.Defer(func() {
				defer func() {
					a = true
				}()
				defer func() {
					b = true
					runtime.Goexit()
				}()
				defer func() {
					c = true
				}()
				runtime.Goexit()
			})
			td.Finish()
		})
		//
		assert.True(t, out.OK)
		assert.Must(t).True(a)
		assert.Must(t).True(b)
		assert.Must(t).True(c)
	})

	var a, b, c bool
	out := sandbox.Run(func() {
		td := &teardown.Teardown{}
		defer td.Finish()
		td.Defer(func() {
			a = true
		})
		td.Defer(func() {
			b = true
			runtime.Goexit()
		})
		td.Defer(func() {
			c = true
		})
	})
	//
	assert.True(t, out.OK)
	assert.Must(t).True(a)
	assert.Must(t).True(b)
	assert.Must(t).True(c)
}

func TestTeardown_Defer_panic(t *testing.T) {
	defer func() { recover() }()
	var a, b, c bool
	const expectedPanicMessage = `boom`

	td := &teardown.Teardown{}
	td.Defer(func() { a = true })
	td.Defer(func() { b = true; panic(expectedPanicMessage) })
	td.Defer(func() { c = true })

	actualPanicValue := func() (r interface{}) {
		defer func() { r = recover() }()
		td.Finish()
		return nil
	}()
	//
	assert.Must(t).True(a)
	assert.Must(t).True(b)
	assert.Must(t).True(c)
	assert.Must(t).Equal(expectedPanicMessage, actualPanicValue)
}

func TestTeardown_Defer_withinCleanup(t *testing.T) {
	var a, b, c bool
	td := &teardown.Teardown{}
	td.Defer(func() {
		a = true
		td.Defer(func() {
			b = true
			td.Defer(func() {
				c = true
			})
		})
	})
	td.Finish()
	//
	assert.Must(t).True(a)
	assert.Must(t).True(b)
	assert.Must(t).True(c)
}

func TestTeardown_Defer_withVariadicArgument(t *testing.T) {
	var total int
	s := testcase.NewSpec(t)
	s.Test("", func(t *testcase.T) {
		t.Defer(func(n int, text ...string) { total++ }, 42)
		t.Defer(func(n int, text ...string) { total++ }, 42, "a")
		t.Defer(func(n int, text ...string) { total++ }, 42, "a", "b")
		t.Defer(func(n int, text ...string) { total++ }, 42, "a", "b", "c")
	})
	s.Finish()
	assert.Must(t).Equal(4, total)
}

func TestTeardown_Defer_withVariadicArgument_argumentPassed(t *testing.T) {
	var total int
	sum := func(v int, ns ...int) {
		total += v
		for _, n := range ns {
			total += n
		}
	}
	s := testcase.NewSpec(t)
	s.Test("", func(t *testcase.T) {
		t.Defer(sum, 1)
		t.Defer(sum, 2, 3)
		t.Defer(sum, 4, 5, 6)
	})
	s.Finish()
	assert.Must(t).Equal(1+2+3+4+5+6, total)
}

func TestTeardown_Defer_runtimeGoexit(t *testing.T) {
	t.Run(`spike`, func(t *testing.T) {
		var ran bool
		defer func() { assert.Must(t).True(ran) }()
		t.Run(``, func(t *testing.T) {
			t.Cleanup(func() { ran = true })
			t.Cleanup(func() { runtime.Goexit() })
		})
	})

	sandbox.Run(func() {
		var ran bool
		defer func() { assert.Must(t).True(ran) }()
		td := &teardown.Teardown{}
		td.Defer(func() { ran = true })
		td.Defer(func() { runtime.Goexit() })
		td.Finish()
		assert.Must(t).True(ran)
	})
}

func TestTeardown_Defer_isThreadSafe(t *testing.T) {
	var (
		td       = &teardown.Teardown{}
		out      = &sync.Map{}
		sampling = runtime.NumCPU() * 42

		start sync.WaitGroup
		wg    sync.WaitGroup
	)

	start.Add(1)
	for i := 0; i < sampling; i++ {
		n := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait()
			td.Defer(func() {
				out.Store(n, struct{}{})
			})
		}()
	}
	t.Log(`begin race condition`)
	start.Done() // begin
	t.Log(`wait for all the register to finish`)
	wg.Wait()
	t.Log(`execute registered teardown functions`)
	td.Finish()

	for i := 0; i < sampling; i++ {
		_, ok := out.Load(i)
		assert.Must(t).True(ok)
	}
}

func TestTeardown_Finish_idempotent(t *testing.T) {
	var count int
	td := &teardown.Teardown{}
	td.Defer(func() { count++ })
	td.Finish()
	td.Finish()
	assert.Must(t).Equal(1, count)
}
