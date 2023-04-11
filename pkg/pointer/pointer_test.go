package pointer_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

type ExampleStruct struct {
	StrPtrField *string
	IntPtrField *int
}

func ExampleOf() {
	_ = ExampleStruct{
		StrPtrField: pointer.Of("42"),
		IntPtrField: pointer.Of(42),
	}
}

func TestOf(tt *testing.T) {
	t := testcase.ToT(tt)
	var value = t.Random.String()
	vptr := pointer.Of(value)
	t.Must.Equal(&value, vptr)
}

func ExampleDeref() {
	var es ExampleStruct
	_ = pointer.Deref(es.StrPtrField)
	_ = pointer.Deref(es.IntPtrField)
}

func TestDeref(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, zero value returned", func(t *testing.T) {
		var str *string
		got := pointer.Deref(str)
		assert.Equal[string](t, "", got)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		got := pointer.Deref(&expected)
		assert.Equal[string](t, expected, got)
	})
}

func ExampleInit() {
	type MyType struct {
		V *string
	}
	var mt MyType

	_ = pointer.Init(&mt.V, func() string {
		return "default value from a lambda"
	})

	_ = pointer.Init(&mt.V, pointer.Of("default value from a pointer"))
}

func TestInit(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, value is constructed from a func", func(t *testing.T) {
		var str *string
		exp := rnd.String()
		got := pointer.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, *str)
	})
	t.Run("on nil value, fallback default value is set as the value", func(t *testing.T) {
		var str *string
		exp := rnd.String()
		got := pointer.Init(&str, &exp)
		assert.Equal[string](t, exp, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, exp, *str)

		t.Run("the used default value can't be mutated with the initialised pointer value", func(t *testing.T) {
			uid := rnd.UUID()
			*str = uid
			assert.NotEqual(t, uid, exp)
		})
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str *string
		str = &expected
		got := pointer.Init(&str, func() string { return "42" })
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, *str)
	})
	t.Run("supports embedded initialisation", func(t *testing.T) {
		expected := rnd.String()
		var str1, str2 *string
		got := pointer.Init(&str1, func() string {
			return pointer.Init(&str2, func() string {
				return expected
			})
		})
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str1)
		assert.Equal[string](t, expected, *str1)
		assert.NotNil(t, str2)
		assert.Equal[string](t, expected, *str2)
	})
}

func TestInit_race(t *testing.T) {
	var str1, str2 *string

	rw := func() { _ = pointer.Init(&str1, func() string { return "42" }) }

	_ = pointer.Init(&str2, func() string { return "42" })
	r := func() { _ = pointer.Init(&str2, func() string { return "42" }) }

	w := func() {
		var str3 *string
		_ = pointer.Init(&str3, func() string { return "42" })
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
		var str *string
		pointer.Init(&str, initFunc)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pointer.Init(&str, initFunc)
		}
	})

	b.Run("[R] while having concurrent access", func(b *testing.B) {
		makeConcurrentAccesses(b)
		var str *string
		_ = pointer.Init(&str, initFunc)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pointer.Init(&str, initFunc)
		}
	})

	b.Run("[W] when init required", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var str *string
			_ = pointer.Init(&str, initFunc)
		}
	})

	b.Run("[W] while having concurrent access", func(b *testing.B) {
		makeConcurrentAccesses(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var str *string
			_ = pointer.Init(&str, initFunc)
		}
	})

	b.Run("[RW]", func(b *testing.B) {
		var str *string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				str = nil
			}
			_ = pointer.Init(&str, initFunc)
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
			_ = pointer.Init(&str, func() string { return "42" })
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
