package zerokit_test

import (
	"context"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func ExampleCoalesce() {
	_ = zerokit.Coalesce("", "", "42") // -> "42"
}

func TestCoalesce(t *testing.T) {
	s := testcase.NewSpec(t)

	var values = testcase.LetValue[[]int](s, nil)

	act := func(t *testcase.T) int {
		return zerokit.Coalesce(values.Get(t)...)
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

	_ = zerokit.Init(&mt.V, func() *string {
		return pointer.Of("default value from a lambda")
	})

	_ = zerokit.Init(&mt.V, pointer.Of(pointer.Of("default value from a pointer")))
}

func TestInit(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, value is constructed from a func() T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zerokit.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on nil value, fallback default value is set as the value", func(t *testing.T) {
		var str *string
		exp := rnd.String()
		got := zerokit.Init(&str, pointer.Of(&exp))
		assert.Equal[string](t, exp, *got)
		assert.NotNil(t, str)
		assert.Equal[string](t, exp, *str)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str *string
		str = &expected
		got := zerokit.Init(&str, func() *string { return pointer.Of("42") })
		assert.Equal[string](t, expected, *got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, *str)
	})
	t.Run("on zero value, value is constructed from a func() T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zerokit.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on zero value, value is constructed from a func() *T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zerokit.Init(&str, func() string { return exp })
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on zero value, fallback default value is set as the value", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got := zerokit.Init(&str, &exp)
		assert.Equal[string](t, exp, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on non zero value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str string
		str = expected
		got := zerokit.Init(&str, func() string { return "42" })
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, str)
	})
	t.Run("supports embedded initialisation", func(t *testing.T) {
		expected := rnd.String()
		var str1, str2 string
		got := zerokit.Init(&str1, func() string {
			return zerokit.Init(&str2, func() string {
				return expected
			})
		})
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str1)
		assert.Equal[string](t, expected, str1)
		assert.NotNil(t, str2)
		assert.Equal[string](t, expected, str2)
	})
	t.Run("when not comparable values are being compared", func(t *testing.T) {
		var v map[string]struct{}
		got := zerokit.Init(&v, func() map[string]struct{} {
			return map[string]struct{}{"42": {}}
		})
		assert.Equal(t, map[string]struct{}{"42": {}}, v)
		assert.Equal(t, map[string]struct{}{"42": {}}, got)
	})
}

func TestInit_atomic(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("int32", func(t *testing.T) {
		var v int32
		exp := int32(rnd.IntN(1024))

		var (
			done  = make(chan struct{})
			ready = make(chan struct{})
		)
		defer close(done)
		defer close(ready)
		go func() {
			var str string
			zerokit.Init(&str, func() string {
				ready <- struct{}{}
				<-done
				return "-"
			})
		}()

		<-ready

		var got int32
		assert.Within(t, time.Second, func(ctx context.Context) {
			got = zerokit.Init(&v, func() int32 { return exp })
		})
		assert.Equal(t, exp, got)
		assert.Equal(t, exp, v)
	})
	t.Run("int64", func(t *testing.T) {
		var v int64
		exp := int64(rnd.IntN(1024))
		got := zerokit.Init(&v, func() int64 { return exp })
		assert.Equal(t, exp, got)
		assert.Equal(t, exp, v)
	})
}

func TestInit_concurrent(t *testing.T) {
	var rnd = random.New(random.CryptoSeed{})
	var vs = make([]int, runtime.NumCPU())
	var dos []func()
	for i := 0; i < runtime.NumCPU(); i++ {
		i := i // dedicate a variable for the closures
		dos = append(dos, func() {
			zerokit.Init(&vs[i], func() int {
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

	rw := func() { _ = zerokit.Init(&str1, func() string { return "42" }) }

	_ = zerokit.Init(&str2, func() string { return "42" })
	r := func() { _ = zerokit.Init(&str2, func() string { return "42" }) }

	w := func() {
		var str3 string
		_ = zerokit.Init(&str3, func() string { return "42" })
	}

	var more []func()
	more = append(more, random.Slice[func()](100, func() func() { return rw })...)
	more = append(more, random.Slice[func()](100, func() func() { return r })...)
	more = append(more, random.Slice[func()](100, func() func() { return w })...)

	var int1 int32
	more = append(more, func() {
		zerokit.Init(&int1, func() int32 {
			return 42
		})
	})

	var int2 int64
	more = append(more, func() {
		zerokit.Init(&int2, func() int64 {
			return 42
		})
	})

	testcase.Race(r, w, more...)
}

func BenchmarkInit(b *testing.B) {
	type Example struct{ V int }

	benchInit[Example](b, func() Example {
		return Example{V: 42}
	})

	benchInit[int32](b, func() int32 {
		return 42
	})

	benchInit[int64](b, func() int64 {
		return 42
	})

	type uncomparable map[string]struct{}
	benchInit[uncomparable](b, func() uncomparable {
		return make(uncomparable)
	})
}

func benchInit[T any](b *testing.B, init func() T) {
	b.Run(reflectkit.SymbolicName(*new(T)), func(b *testing.B) {
		b.Run("R when init is not required", func(b *testing.B) {
			var v T
			zerokit.Init(&v, init)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("R when init is not required", func(b *testing.B) {
			var v T
			zerokit.Init(&v, init)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("R while having concurrent read access", func(b *testing.B) {
			makeConcurrentReadsAccesses[T](b, init)
			var v T
			_ = zerokit.Init(&v, init)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("R while having concurrent write access", func(b *testing.B) {
			makeConcurrentAccesses[T](b, init)
			var v T
			_ = zerokit.Init(&v, init)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("W when init required", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var v T
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("W while having concurrent access", func(b *testing.B) {
			makeConcurrentAccesses[T](b, init)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var v T
				_ = zerokit.Init(&v, init)
			}
		})

		b.Run("RW", func(b *testing.B) {
			var v T
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					v = *new(T) // zero it out
				}
				_ = zerokit.Init(&v, init)
			}
		})
	})
}

func makeConcurrentReadsAccesses[T any](tb testing.TB, init func() T) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var (
		ready int32
		v     T
		blk   = func() { _ = zerokit.Init(&v, init) }
	)
	blk()
	go func() {
		more := random.Slice[func()](runtime.NumCPU()*2, func() func() { return blk })
		atomic.AddInt32(&ready, 1)
		for {
			if ctx.Err() != nil {
				break
			}
			testcase.Race(blk, blk, more...)
		}
	}()
	for {
		if atomic.LoadInt32(&ready) != 0 {
			break
		}
	}
}

func makeConcurrentAccesses[T any](tb testing.TB, init func() T) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var ready int32
	go func() {
		blk := func() {
			var v T
			_ = zerokit.Init(&v, init)
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
