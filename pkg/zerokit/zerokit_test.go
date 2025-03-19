package zerokit_test

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

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

func BenchmarkCoalesce(b *testing.B) {
	b.Run("primitive", func(b *testing.B) {
		b.Run("string", func(b *testing.B) {
			const defaultValue = "foo bar baz"

			for range b.N {
				zerokit.Coalesce("", defaultValue)
			}
		})
		b.Run("int", func(b *testing.B) {
			const defaultValue = 42

			for range b.N {
				zerokit.Coalesce(0, defaultValue)
			}
		})
		b.Run("float", func(b *testing.B) {
			const defaultValue = 42.42

			for range b.N {
				zerokit.Coalesce(0.0, defaultValue)
			}
		})
	})
	b.Run("comparable", func(b *testing.B) {
		b.Run("time.Time", func(b *testing.B) {
			var defaultTime = time.Now()
			b.ResetTimer()

			for range b.N {
				zerokit.Coalesce(time.Time{}, defaultTime)
			}
		})
	})
}

type StubIsZero struct {
	ZeroItIs bool
	V        int
}

func (s StubIsZero) IsZero() bool {
	return s.ZeroItIs
}

func ExampleInit() {
	type MyType struct {
		V string
	}
	var mt MyType

	_ = zerokit.Init(&mt.V, func() string {
		return "default value from a lambda"
	})

	var defaultValue = "default value from a shared variable"
	_ = zerokit.Init(&mt.V, &defaultValue)
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

func ExampleInitErr() {
	var val string
	got, err := zerokit.InitErr(&val, func() (string, error) {
		return "foo", fmt.Errorf("some error might occur, it will be handled")
	})
	_, _ = got, err
}

func TestInitErr(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, value is constructed from a func() T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got, err := zerokit.InitErr(&str, func() (string, error) { return exp, nil })
		assert.NoError(t, err)
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str *string
		str = &expected
		got, err := zerokit.InitErr(&str, func() (*string, error) { return pointer.Of("42"), nil })
		assert.NoError(t, err)
		assert.Equal[string](t, expected, *got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, *str)
	})
	t.Run("on zero value, value is constructed from a func() T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got, err := zerokit.InitErr(&str, func() (string, error) { return exp, nil })
		assert.NoError(t, err)
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on zero value, value is constructed from a func() *T", func(t *testing.T) {
		var str string
		exp := rnd.String()
		got, err := zerokit.InitErr(&str, func() (string, error) { return exp, nil })
		assert.NoError(t, err)
		assert.Equal[string](t, exp, got)
		assert.Equal[string](t, exp, str)
	})
	t.Run("on non zero value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		var str string
		str = expected
		got, err := zerokit.InitErr(&str, func() (string, error) { return "42", nil })
		assert.NoError(t, err)
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str)
		assert.Equal[string](t, expected, str)
	})
	t.Run("supports embedded initialisation", func(t *testing.T) {
		expected := rnd.String()
		var str1, str2 string
		got, err := zerokit.InitErr(&str1, func() (string, error) {
			return zerokit.InitErr(&str2, func() (string, error) {
				return expected, nil
			})
		})
		assert.NoError(t, err)
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str1)
		assert.Equal[string](t, expected, str1)
		assert.NotNil(t, str2)
		assert.Equal[string](t, expected, str2)
	})
	t.Run("supports embedded initialisation with Init", func(t *testing.T) {
		expected := rnd.String()
		var str1, str2 string
		got, err := zerokit.InitErr(&str1, func() (string, error) {
			return zerokit.Init(&str2, func() string {
				return expected
			}), nil
		})
		assert.NoError(t, err)
		assert.Equal[string](t, expected, got)
		assert.NotNil(t, str1)
		assert.Equal[string](t, expected, str1)
		assert.NotNil(t, str2)
		assert.Equal[string](t, expected, str2)
	})
	t.Run("when not comparable values are being compared", func(t *testing.T) {
		var v map[string]struct{}
		got, err := zerokit.InitErr(&v, func() (map[string]struct{}, error) {
			return map[string]struct{}{"42": {}}, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"42": {}}, v)
		assert.Equal(t, map[string]struct{}{"42": {}}, got)
	})
	t.Run("when error occurs, it is propagaded back", func(t *testing.T) {
		var v string
		var expVal = rnd.String()
		var expErr error = rnd.Error()
		got, err := zerokit.InitErr(&v, func() (string, error) {
			return expVal, expErr
		})
		assert.ErrorIs(t, err, expErr)
		assert.Equal(t, got, expVal)
	})
	t.Run("when error occurs, then it is not persisted", func(t *testing.T) {
		var v string
		var expVal = rnd.String()
		var expErr error = rnd.Error()
		var once sync.Once
		var init = func() (string, error) {
			var err error
			once.Do(func() { err = expErr })
			return expVal, err
		}
		_, err := zerokit.InitErr(&v, init)
		assert.ErrorIs(t, err, expErr)

		got, err := zerokit.InitErr(&v, init)
		assert.NoError(t, err)
		assert.Equal(t, got, expVal)
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

func TestIsZero(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		var i int
		assert.True(t, zerokit.IsZero(i))
		i = 42
		assert.False(t, zerokit.IsZero(i))
	})

	t.Run("string", func(t *testing.T) {
		var s string
		assert.True(t, zerokit.IsZero(s))
		s = "hello"
		assert.False(t, zerokit.IsZero(s))
	})

	t.Run("pointer", func(t *testing.T) {
		var p *int
		assert.True(t, zerokit.IsZero(p))
		i := 42
		p = &i
		assert.False(t, zerokit.IsZero(p))
	})

	t.Run("slice", func(t *testing.T) {
		var s []int
		assert.True(t, zerokit.IsZero(s))
		s = []int{1, 2, 3}
		assert.False(t, zerokit.IsZero(s))
	})

	t.Run("map", func(t *testing.T) {
		var m map[string]int
		assert.True(t, zerokit.IsZero(m))
		m = map[string]int{"foo": 42}
		assert.False(t, zerokit.IsZero(m))
	})

	t.Run("smoke test with special types", func(t *testing.T) {
		var netIP net.IP // Cmp
		assert.True(t, zerokit.IsZero(netIP))
		assert.False(t, zerokit.IsZero(net.IPv4(0, 0, 0, 0)))
		var bigInt big.Int // Cmp
		assert.True(t, zerokit.IsZero(bigInt))
		assert.False(t, zerokit.IsZero(big.NewInt(0)))
		var timeTime time.Time // IsZero
		assert.True(t, zerokit.IsZero(timeTime))
		assert.False(t, zerokit.IsZero(time.Unix(0, 0)))
	})
}

func BenchmarkInit_vsV(b *testing.B) {
	b.Run("zerokit.V[int]", func(b *testing.B) {
		v := zerokit.V[int]{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v.Get()
		}
	})
	b.Run("zerokit.Init[int]", func(b *testing.B) {
		var val int
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			zerokit.Init(&val, func() int {
				return 0
			})
		}
	})
	b.Run("zerokit.V[[]string]", func(b *testing.B) {
		v := zerokit.V[[]string]{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v.Get()
		}
	})
	b.Run("zerokit.Init[[]string]]", func(b *testing.B) {
		var val []string
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			zerokit.Init(&val, func() []string {
				return []string{}
			})
		}
	})
}

func ExampleV() {
	type MyStructType struct {
		fields    zerokit.V[*[]string]
		foundKeys zerokit.V[map[string]struct{}]
	}

	var mst MyStructType
	mst.foundKeys.Get()
}

func TestV(t *testing.T) {
	t.Run("value type", func(t *testing.T) {
		v1 := zerokit.V[int]{}
		assert.Equal(t, 0, v1.Get())

		v2 := zerokit.V[string]{}
		assert.Equal(t, "", v2.Get())

		v3 := zerokit.V[bool]{}
		assert.Equal(t, false, v3.Get())
	})
	t.Run("slice type", func(t *testing.T) {
		v1 := zerokit.V[[]int]{}
		assert.Equal(t, 0, len(v1.Get()))
		assert.NotNil(t, v1.Get())
		v1.Set(append(v1.Get(), 42))
		assert.Equal(t, []int{42}, v1.Get())

		v2 := zerokit.V[[]string]{}
		assert.Equal(t, 0, len(v2.Get()))
		assert.NotNil(t, v2.Get())
		v2.Set(append(v2.Get(), "foo"))
		assert.Equal(t, []string{"foo"}, v2.Get())
	})
	t.Run("map type", func(t *testing.T) {
		v1 := zerokit.V[map[string]int]{}
		assert.Equal(t, 0, len(v1.Get()))
		assert.NotNil(t, v1.Get())
		v1.Get()["foo"] = 42
		assert.Equal(t, map[string]int{"foo": 42}, v1.Get())
	})
	t.Run("type with init", func(t *testing.T) {
		v := zerokit.V[TypeWithInit]{}
		assert.Equal(t, 42, v.Get().X)
	})
	t.Run("pointer type", func(t *testing.T) {
		v := zerokit.V[*int]{}
		assert.Equal(t, pointer.Of(0), v.Get())
		*v.Get() = 42
		assert.Equal(t, pointer.Of(42), v.Get())
	})
	t.Run(".Ptr()", func(t *testing.T) {
		v := zerokit.V[string]{}
		const val = "foo/bar/baz"
		*v.Ptr() = val
		assert.Equal(t, val, v.Get())
	})
}

type TypeWithInit struct {
	X int
}

func (v *TypeWithInit) Init() {
	v.X = 42
}
