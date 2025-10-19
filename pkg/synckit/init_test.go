package synckit_test

import (
	"strings"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type TypeWithInit struct {
	X int
}

func (v *TypeWithInit) Init() {
	v.X = 42
}

func TestInit(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("sync.RWMutex", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			var rwm sync.RWMutex
			exp := t.Random.Int()

			var val int

			got := synckit.Init(&rwm, &val, func() int {
				return exp
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)

			t.Random.Repeat(3, 7, func() {
				got = synckit.Init(&rwm, &val, func() int { panic("boom") })
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("race", func(t *testcase.T) {
			var rwm sync.RWMutex
			exp := t.Random.Int()
			mk := func() int { return exp }
			var val int

			testcase.Race(func() {
				synckit.Init(&rwm, &val, mk)
			}, func() {
				synckit.Init(&rwm, &val, mk)
			})
		})
	})

	s.Context("sync.Mutex", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()

			var val int

			got := synckit.Init(&m, &val, func() int {
				return exp
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)

			t.Random.Repeat(3, 7, func() {
				got = synckit.Init(&m, &val, func() int { panic("boom") })
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("race", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()
			mk := func() int { return exp }
			var val int

			testcase.Race(func() {
				synckit.Init(&m, &val, mk)
			}, func() {
				synckit.Init(&m, &val, mk)
			})
		})
	})

	s.Context("sync.Once", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			var o sync.Once

			exp := t.Random.Int()
			var val int

			got := synckit.Init(&o, &val, func() int {
				return exp
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)

			t.Random.Repeat(3, 7, func() {
				got = synckit.Init(&o, &val, func() int { panic("boom") })
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("race", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()
			mk := func() int { return exp }
			var val int

			testcase.Race(func() {
				synckit.Init(&m, &val, mk)
			}, func() {
				synckit.Init(&m, &val, mk)
			})
		})
	})
}

func TestInitErr(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("sync.RWMutex", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			var rwm sync.RWMutex
			exp := t.Random.Int()

			var val int

			got, err := synckit.InitErr(&rwm, &val, func() (int, error) {
				return exp, nil
			})
			assert.NoError(t, err)

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)

			t.Random.Repeat(3, 7, func() {
				got, err = synckit.InitErr(&rwm, &val, func() (int, error) { panic("boom") })
				assert.NoError(t, err)
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("err", func(t *testcase.T) {
			var m sync.RWMutex
			exp := t.Random.Int()
			expErr := t.Random.Error()
			var val int
			got, err := synckit.InitErr(&m, &val, func() (int, error) {
				return exp, expErr
			})
			assert.ErrorIs(t, expErr, err)
			got, err = synckit.InitErr(&m, &val, func() (int, error) {
				return exp, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("race", func(t *testcase.T) {
			var rwm sync.RWMutex
			exp := t.Random.Int()
			mk := func() (int, error) { return exp, nil }
			var val int

			testcase.Race(func() {
				synckit.InitErr(&rwm, &val, mk)
			}, func() {
				synckit.InitErr(&rwm, &val, mk)
			})
		})
	})

	s.Context("sync.Mutex", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()

			var val int

			got, err := synckit.InitErr(&m, &val, func() (int, error) {
				return exp, nil
			})
			assert.NoError(t, err)

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)

			t.Random.Repeat(3, 7, func() {
				got, err = synckit.InitErr(&m, &val, func() (int, error) { panic("boom") })
				assert.NoError(t, err)
			})

			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("err", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()
			expErr := t.Random.Error()
			var val int
			got, err := synckit.InitErr(&m, &val, func() (int, error) {
				return exp, expErr
			})
			assert.ErrorIs(t, expErr, err)
			got, err = synckit.InitErr(&m, &val, func() (int, error) {
				return exp, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
			assert.Equal(t, exp, val)
		})

		s.Test("race", func(t *testcase.T) {
			var m sync.Mutex
			exp := t.Random.Int()
			mk := func() (int, error) { return exp, nil }
			var val int

			testcase.Race(func() {
				synckit.InitErr(&m, &val, mk)
			}, func() {
				synckit.InitErr(&m, &val, mk)
			})
		})
	})
}

func Benchmark_init_once(b *testing.B) {
	b.Run("Init", func(b *testing.B) {
		initBenchmarkCases(b, func() *sync.Once {
			return &sync.Once{}
		}, func(l *sync.Once, ptr *string, init func() string) {
			synckit.Init(l, ptr, init)
		})

		b.Run("*sync.Once raw/real", func(b *testing.B) {
			var v string
			rnd := random.New(random.CryptoSeed{})
			val := rnd.HexN(8)
			var init = func() string {
				return val
			}
			var once sync.Once
			b.ResetTimer()
			for range b.N {
				_ = InitOncePure(&once, &v, init)
			}
		})

		initBenchmarkCases(b, func() *sync.Mutex {
			return &sync.Mutex{}
		}, func(l *sync.Mutex, ptr *string, init func() string) {
			synckit.Init(l, ptr, init)
		})

		initBenchmarkCases(b, func() *sync.RWMutex {
			return &sync.RWMutex{}
		}, func(l *sync.RWMutex, ptr *string, init func() string) {
			synckit.Init(l, ptr, init)
		})
	})

	b.Run("InitErr", func(b *testing.B) {
		initBenchmarkCases(b, func() *sync.Mutex {
			return &sync.Mutex{}
		}, func(l *sync.Mutex, ptr *string, init func() string) {
			synckit.InitErr(l, ptr, func() (string, error) {
				return init(), nil
			})
		})

		initBenchmarkCases(b, func() *sync.RWMutex {
			return &sync.RWMutex{}
		}, func(l *sync.RWMutex, ptr *string, init func() string) {
			synckit.InitErr(l, ptr, func() (string, error) {
				return init(), nil
			})
		})
	})
}

func InitOncePure(l *sync.Once, ptr *string, init func() string) string {
	l.Do(func() { *ptr = init() })
	return *ptr
}

func initBenchmarkCases[L *sync.RWMutex | *sync.Mutex | *sync.Once](b *testing.B,
	mkl func() L,
	subject func(L L, ptr *string, init func() string),
) {
	name := reflectkit.TypeOf[L]().String()
	rnd := random.New(random.CryptoSeed{})
	val := rnd.HexN(7)
	var init = func() string {
		return val
	}

	name += strings.Repeat("-", 15-len([]rune(name)))

	b.Run(name, func(b *testing.B) {
		b.Run("real-", func(b *testing.B) {
			var l = mkl()
			var v string
			b.ResetTimer()
			for range b.N {
				subject(l, &v, init)
			}
		})
		b.Run("worse", func(b *testing.B) {
			for range b.N {
				var l = mkl()
				var v string
				subject(l, &v, init)
			}
		})
	})
}

func TestInitErrWL(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("sync.Mutex", func(t *testcase.T) {
		var m sync.Mutex
		exp := t.Random.Int()

		var val int

		got, err := synckit.InitErrWL(&m, &val, func() (int, error) {
			return exp, nil
		})
		assert.NoError(t, err)

		assert.Equal(t, exp, got)
		assert.Equal(t, exp, val)

		t.Random.Repeat(3, 7, func() {
			got, err = synckit.InitErr(&m, &val, func() (int, error) { panic("boom") })
			assert.NoError(t, err)
		})

		assert.Equal(t, exp, got)
		assert.Equal(t, exp, val)
	})

	s.Test("sync.RWMutex", func(t *testcase.T) {
		var m sync.RWMutex
		exp := t.Random.Int()

		var val int

		got, err := synckit.InitErrWL(&m, &val, func() (int, error) {
			return exp, nil
		})
		assert.NoError(t, err)

		assert.Equal(t, exp, got)
		assert.Equal(t, exp, val)

		t.Random.Repeat(3, 7, func() {
			got, err = synckit.InitErr(&m, &val, func() (int, error) { panic("boom") })
			assert.NoError(t, err)
		})

		assert.Equal(t, exp, got)
		assert.Equal(t, exp, val)
	})

	s.Test("rainy", func(t *testcase.T) {
		var m sync.Mutex
		exp := t.Random.Int()
		expErr := t.Random.Error()
		var val int
		got, err := synckit.InitErr(&m, &val, func() (int, error) {
			return exp, expErr
		})
		assert.ErrorIs(t, expErr, err)
		got, err = synckit.InitErr(&m, &val, func() (int, error) {
			return exp, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
		assert.Equal(t, exp, val)
	})

	s.Test("race", func(t *testcase.T) {
		var m sync.Mutex
		exp := t.Random.Int()
		mk := func() (int, error) { return exp, nil }
		var val int

		testcase.Race(func() {
			synckit.InitErr(&m, &val, mk)
		}, func() {
			synckit.InitErr(&m, &val, mk)
		})
	})
}
