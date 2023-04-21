package zeroutil_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/tasker"
	"github.com/adamluzsi/testcase/let"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/frameless/pkg/zeroutil"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func ExampleCoalesce() {
	_ = zeroutil.Coalesce("", "", "42") // -> "42"
}

func TestCoalesce(t *testing.T) {
	s := testcase.NewSpec(t)

	var values = testcase.LetValue[[]int](s, nil)

	act := func(t *testcase.T) int {
		return zeroutil.Coalesce(values.Get(t)...)
	}

	s.When("values are empty", func(s *testcase.Spec) {
		values.LetValue(s, nil)

		s.Then("zero value is returned", func(t *testcase.T) {
			t.Must.Equal(*new(int), act(t))
		})
	})

	s.When("values have a single non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{expected.Get(t)}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})

	s.When("values have multiple values, but the first one is the non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{expected.Get(t), 0, 0}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})

	s.When("values have multiple values, but not the first one is the non-zero value", func(s *testcase.Spec) {
		expected := let.Int(s)

		values.Let(s, func(t *testcase.T) []int {
			return []int{0, expected.Get(t), 0}
		})

		s.Then("the non-zero value is returned", func(t *testcase.T) {
			t.Must.Equal(expected.Get(t), act(t))
		})
	})
}

func ExampleInit() {
	type MyType struct {
		V *string
	}
	var mt MyType

	_ = zeroutil.Init(&mt.V, func() *string {
		return pointer.Of("default value from a lambda")
	})

	_ = zeroutil.Init(&mt.V, pointer.Of(pointer.Of("default value from a pointer")))
}

func TestInit(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, value is constructed from a func() T", func(t *testing.T) {
		var str *string
		exp := rnd.String()
		got := zeroutil.Init(&str, func() *string { return &exp })
		assert.Equal[string](t, exp, *got)
		assert.Equal[string](t, exp, *str)
	})
	t.Run("on nil value, fallback default value is set as the value", func(t *testing.T) {
		var str *string
		exp := rnd.String()
		got := zeroutil.Init(&str, pointer.Of(&exp))
		assert.Equal[string](t, exp, *got)
		assert.NotNil(t, str)
		assert.Equal[string](t, exp, *str)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str *string
		str = &expected
		got := zeroutil.Init(&str, func() *string { return pointer.Of("42") })
		assert.Equal[string](t, expected, *got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, *str)
	})
	t.Run("on zero value, value is constructed from a func() T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zeroutil.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on zero value, value is constructed from a func() *T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zeroutil.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on zero value, fallback default value is set as the value", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zeroutil.Init(&str, &exp)
		assert.Equal[string](t, exp, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on non zero value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str string
		str = expected
		got := zeroutil.Init(&str, func() string { return "42" })
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, str)
	})
	t.Run("supports embedded initialisation", func(t *testing.T) {
		expected := rnd.String()
		var str1, str2 string
		got := zeroutil.Init(&str1, func() string {
			return zeroutil.Init(&str2, func() string {
				return expected
			})
		})
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str1)
		assert.Equal[string](t, expected, str1)
		assert.NotNil(t, str2)
		assert.Equal[string](t, expected, str2)
	})
}

func TestInit_concurrent(t *testing.T) {
	var rnd = random.New(random.CryptoSeed{})
	var vs = make([]int, runtime.NumCPU())
	var dos []func()
	for i := 0; i < runtime.NumCPU(); i++ {
		i := i // dedicate a variable for the closures
		dos = append(dos, func() {
			zeroutil.Init(&vs[i], func() int {
				time.Sleep(time.Second)
				return rnd.Int()
			})
		})
	}
	assert.Within(t, time.Second+500*time.Millisecond, func(ctx context.Context) {
		assert.NoError(t, tasker.Concurrence(dos...).Run(ctx))
	})
}

func TestInit_race(t *testing.T) {
	var str1, str2 string

	rw := func() { _ = zeroutil.Init(&str1, func() string { return "42" }) }

	_ = zeroutil.Init(&str2, func() string { return "42" })
	r := func() { _ = zeroutil.Init(&str2, func() string { return "42" }) }

	w := func() {
		var str3 string
		_ = zeroutil.Init(&str3, func() string { return "42" })
	}

	var more []func()
	more = append(more, random.Slice[func()](100, func() func() { return rw })...)
	more = append(more, random.Slice[func()](100, func() func() { return r })...)
	more = append(more, random.Slice[func()](100, func() func() { return w })...)
	testcase.Race(r, w, more...)
}

func BenchmarkInit(b *testing.B) {
	initFunc := func() string { return "42" }

	b.Run("[R] when init is not required", func(b *testing.B) {
		var str string
		zeroutil.Init(&str, initFunc)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = zeroutil.Init(&str, initFunc)
		}
	})

	b.Run("[R] while having concurrent access", func(b *testing.B) {
		makeConcurrentAccesses(b)
		var str string
		_ = zeroutil.Init(&str, initFunc)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = zeroutil.Init(&str, initFunc)
		}
	})

	b.Run("[W] when init required", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var str string
			_ = zeroutil.Init(&str, initFunc)
		}
	})

	b.Run("[W] while having concurrent access", func(b *testing.B) {
		makeConcurrentAccesses(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var str string
			_ = zeroutil.Init(&str, initFunc)
		}
	})

	b.Run("[RW]", func(b *testing.B) {
		var str string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				str = "" // zero it out
			}
			_ = zeroutil.Init(&str, initFunc)
		}
	})
}

func makeConcurrentAccesses(tb testing.TB) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var ready int32
	go func() {
		blk := func() {
			var str *string
			_ = zeroutil.Init(&str, func() *string { return pointer.Of("42") })
		}
		more := random.Slice[func()](runtime.NumCPU()*2, func() func() { return blk })
		atomic.AddInt32(&ready, 1)
		func() {
			for {
				if ctx.Err() != nil {
					break
				}
				testcase.Race(blk, blk, more...)
			}
		}()
	}()
	for {
		if atomic.LoadInt32(&ready) != 0 {
			break
		}
	}
}
