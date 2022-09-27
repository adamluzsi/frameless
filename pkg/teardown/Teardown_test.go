package teardown_test

import (
	"runtime"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/pkg/teardown"
	"github.com/adamluzsi/testcase/sandbox"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func TestTeardown_Defer_order(t *testing.T) {
	td := &teardown.Teardown{}
	var res []int
	td.Defer(func() error { res = append(res, 3); return nil })
	td.Defer(func() error { res = append(res, 2); return nil })
	td.Defer(func() error { res = append(res, 1); return nil })
	td.Defer(func() error { res = append(res, 0); return nil })
	assert.NoError(t, td.Finish())
	assert.Equal(t, []int{0, 1, 2, 3}, res)
}

func TestTeardown_Defer_commonFunctionSignatures(t *testing.T) {
	td := &teardown.Teardown{}
	var res []int
	td.Defer(func() error { res = append(res, 1); return nil })
	td.Defer(func() error { res = append(res, 0); return nil })
	assert.NoError(t, td.Finish())
	assert.Equal(t, []int{0, 1}, res)
}

func TestTeardown_Defer_smoke(t *testing.T) {
	var a, b, c bool
	out := sandbox.Run(func() {
		td := &teardown.Teardown{}
		defer td.Finish()
		td.Defer(func() error {
			a = true
			return nil
		})
		td.Defer(func() error {
			b = true
			return nil
		})
		td.Defer(func() error {
			c = true
			return nil
		})
	})
	//
	assert.True(t, out.OK)
	assert.True(t, a)
	assert.True(t, b)
	assert.True(t, c)
}

func TestTeardown_Defer_panic(t *testing.T) {
	defer func() { recover() }()
	var a, b, c bool
	const expectedPanicMessage = `boom`

	td := &teardown.Teardown{}
	td.Defer(func() error { a = true; return nil })
	td.Defer(func() error { b = true; panic(expectedPanicMessage); return nil })
	td.Defer(func() error { c = true; return nil })

	actualPanicValue := func() (r interface{}) {
		defer func() { r = recover() }()
		assert.NoError(t, td.Finish())
		return nil
	}()
	//
	assert.True(t, a)
	assert.True(t, b)
	assert.True(t, c)
	assert.Equal(t, expectedPanicMessage, actualPanicValue)
}

func TestTeardown_Defer_withinCleanup(t *testing.T) {
	var a, b, c bool
	td := &teardown.Teardown{}
	td.Defer(func() error {
		a = true
		td.Defer(func() error {
			b = true
			td.Defer(func() error {
				c = true
				return nil
			})
			return nil
		})
		return nil
	})
	td.Finish()
	//
	assert.True(t, a)
	assert.True(t, b)
	assert.True(t, c)
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
	assert.Equal(t, 4, total)
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
	assert.Equal(t, 1+2+3+4+5+6, total)
}

func TestTeardown_Defer_runtimeGoexit(t *testing.T) {
	t.Run(`spike`, func(t *testing.T) {
		var ran bool
		defer func() { assert.True(t, ran) }()
		t.Run(``, func(t *testing.T) {
			t.Cleanup(func() { ran = true })
			t.Cleanup(func() { runtime.Goexit() })
		})
	})

	sandbox.Run(func() {
		var ran bool
		defer func() { assert.True(t, ran) }()
		td := &teardown.Teardown{}
		td.Defer(func() error { ran = true; return nil })
		td.Defer(func() error { runtime.Goexit(); return nil })
		assert.NoError(t, td.Finish())
		assert.True(t, ran)
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
			td.Defer(func() error {
				out.Store(n, struct{}{})
				return nil
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
		assert.True(t, ok)
	}
}

func TestTeardown_Finish_idempotent(t *testing.T) {
	var count int
	td := &teardown.Teardown{}
	td.Defer(func() error { count++; return nil })
	assert.NoError(t, td.Finish())
	assert.NoError(t, td.Finish())
	assert.Equal(t, 1, count)
}
