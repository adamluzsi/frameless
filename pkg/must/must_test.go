package must_test

import (
	"bytes"
	htmlTemplate "html/template"
	"io"
	"regexp"
	"testing"
	txtTemplate "text/template"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleMust_regexp() {
	must.Must(regexp.Compile(`^\w+$`))
}

func ExampleMust_textTemplate() {
	tmpl := must.Must(txtTemplate.New("").Parse(`{{.Label}}`))
	_ = tmpl
}

func ExampleMust_htmlTemplate() {
	tmpl := must.Must(htmlTemplate.New("").Parse(`<div>{{.Label}}</div>`))
	_ = tmpl
}

func ExampleMust_ioReadAll() {
	in := bytes.NewReader([]byte("Hello, world!"))
	data := must.Must(io.ReadAll(in))
	_ = data
}

func ExampleMust() {
	fn := func() (int, error) { return 42, nil }
	val := must.Must(fn())
	_ = val
}

func TestMust(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			exp = rnd.String()
			got string
		)
		assert.NotPanic(t, func() {
			got = must.Must(func() (string, error) { return exp, nil }())
		})
		assert.Equal(t, exp, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			exp = rnd.Error()
			got error
		)
		out := assert.Panic(t, func() {
			_ = must.Must(func() (int, error) { return rnd.Int(), exp }())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func ExampleMust0() {
	fn := errorkit.NullErrFunc
	must.Must0(fn())
}

func TestMust0(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		assert.NotPanic(t, func() {
			must.Must0(errorkit.NullErrFunc())
		})
	})
	t.Run("rainy", func(t *testing.T) {
		var exp = rnd.Error()
		out := assert.Panic(t, func() {
			must.Must0(func() error { return exp }())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func ExampleMust2() {
	fn := func() (string, int, error) { return "the answer is", 42, nil }
	a, b := must.Must2(fn())
	_, _ = a, b
}

func TestMust2(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
		)
		assert.NotPanic(t, func() {
			gotA, gotB = must.Must2(func() (string, int, error) { return expA, expB, nil }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			exp = rnd.Error()
			got error
		)
		out := assert.Panic(t, func() {
			_, _ = must.Must2(func() (string, int, error) { return rnd.String(), rnd.Int(), exp }())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func ExampleMust3() {
	fn := func() (string, int, bool, error) { return "the answer is", 42, true, nil }
	a, b, c := must.Must3(fn())
	_, _, _ = a, b, c
}

func TestMust3(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC = must.Must3(func() (string, int, float32, error) { return expA, expB, expC, nil }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			exp = rnd.Error()
			got error
		)
		out := assert.Panic(t, func() {
			_, _, _ = must.Must3(func() (string, int, float32, error) { return rnd.String(), rnd.Int(), rnd.Float32(), exp }())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func ExampleMust4() {
	fn := func() (string, int, bool, float32, error) { return "the answer is", 42, true, 42.42, nil }
	a, b, c, d := must.Must4(fn())
	_, _, _, _ = a, b, c, d
}

func TestMust4(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
			expD = rnd.Time()
			gotD time.Time
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC, gotD = must.Must4(func() (string, int, float32, time.Time, error) { return expA, expB, expC, expD, nil }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
		assert.Equal(t, expD, gotD)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			exp = rnd.Error()
			got error
		)
		out := assert.Panic(t, func() {
			_, _, _, _ = must.Must4(func() (string, int, float32, time.Time, error) {
				return rnd.String(), rnd.Int(), rnd.Float32(), rnd.Time(), exp
			}())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func ExampleMust5() {
	fn := func() (string, int, bool, float32, float64, error) {
		return "the answer is", 42, true, 42.42, 24.24, nil
	}
	a, b, c, d, e := must.Must5(fn())
	_, _, _, _, _ = a, b, c, d, e
}

func TestMust5(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
			expD = rnd.Time()
			gotD time.Time
			expE = rnd.Float64()
			gotE float64
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC, gotD, gotE = must.Must5(func() (string, int, float32, time.Time, float64, error) { return expA, expB, expC, expD, expE, nil }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
		assert.Equal(t, expD, gotD)
		assert.Equal(t, expE, gotE)
	})
	t.Run("rainy", func(t *testing.T) {
		var (
			exp = rnd.Error()
			got error
		)
		out := assert.Panic(t, func() {
			_, _, _, _, _ = must.Must5(func() (string, int, float32, time.Time, float64, error) {
				return rnd.String(), rnd.Int(), rnd.Float32(), rnd.Time(), rnd.Float64(), exp
			}())
		})
		got, ok := out.(error)
		assert.True(t, ok, "Expected to get back an error value as panic's value")
		assert.ErrorIs(t, exp, got)
	})
}

func TestOK(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			exp = rnd.String()
			got string
		)
		assert.NotPanic(t, func() {
			got = must.OK(func() (string, bool) { return exp, true }())
		})
		assert.Equal(t, exp, got)
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			_ = must.OK(func() (int, bool) { return rnd.Int(), false }())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}

func ExampleOK0() {
	fn := func() bool { return true }
	must.OK0(fn())
}

func TestOK0(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		assert.NotPanic(t, func() {
			must.OK0(func() bool { return true }())
		})
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			must.OK0(func() bool { return false }())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}

func ExampleOK2() {
	fn := func() (string, int, bool) { return "the answer is", 42, true }
	a, b := must.OK2(fn())
	_, _ = a, b
}

func TestOK2(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
		)
		assert.NotPanic(t, func() {
			gotA, gotB = must.OK2(func() (string, int, bool) { return expA, expB, true }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			_, _ = must.OK2(func() (string, int, bool) { return rnd.String(), rnd.Int(), false }())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}

func ExampleOK3() {
	fn := func() (string, int, bool, bool) { return "the answer is", 42, true, true }
	a, b, c := must.OK3(fn())
	_, _, _ = a, b, c
}

func TestOK3(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC = must.OK3(func() (string, int, float32, bool) { return expA, expB, expC, true }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			_, _, _ = must.OK3(func() (string, int, float32, bool) { return rnd.String(), rnd.Int(), rnd.Float32(), false }())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}

func ExampleOK4() {
	fn := func() (string, int, bool, float32, bool) { return "the answer is", 42, true, 42.42, true }
	a, b, c, d := must.OK4(fn())
	_, _, _, _ = a, b, c, d
}

func TestOK4(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
			expD = rnd.Time()
			gotD time.Time
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC, gotD = must.OK4(func() (string, int, float32, time.Time, bool) { return expA, expB, expC, expD, true }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
		assert.Equal(t, expD, gotD)
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			_, _, _, _ = must.OK4(func() (string, int, float32, time.Time, bool) {
				return rnd.String(), rnd.Int(), rnd.Float32(), rnd.Time(), false
			}())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}

func ExampleOK5() {
	fn := func() (string, int, bool, float32, float64, bool) {
		return "the answer is", 42, true, 42.42, 24.24, true
	}
	a, b, c, d, e := must.OK5(fn())
	_, _, _, _, _ = a, b, c, d, e
}

func TestOK5(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			expA = rnd.String()
			gotA string
			expB = rnd.Int()
			gotB int
			expC = rnd.Float32()
			gotC float32
			expD = rnd.Time()
			gotD time.Time
			expE = rnd.Float64()
			gotE float64
		)
		assert.NotPanic(t, func() {
			gotA, gotB, gotC, gotD, gotE = must.OK5(func() (string, int, float32, time.Time, float64, bool) { return expA, expB, expC, expD, expE, true }())
		})
		assert.Equal(t, expA, gotA)
		assert.Equal(t, expB, gotB)
		assert.Equal(t, expC, gotC)
		assert.Equal(t, expD, gotD)
		assert.Equal(t, expE, gotE)
	})
	t.Run("rainy", func(t *testing.T) {
		got := assert.Panic(t, func() {
			_, _, _, _, _ = must.OK5(func() (string, int, float32, time.Time, float64, bool) {
				return rnd.String(), rnd.Int(), rnd.Float32(), rnd.Time(), rnd.Float64(), false
			}())
		})
		assert.NotNil(t, got)
		assert.Equal[any](t, got, must.ErrNotOK)
	})
}
