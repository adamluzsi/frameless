package synckit_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

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

func makeConcurrentReadsAccesses[T any](tb testing.TB, init func(*T) T) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var (
		ready int32
		v     T
		blk   = func() { _ = init(&v) }
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

func makeConcurrentAccesses[T any](tb testing.TB, init func(*T) T) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var ready int32
	go func() {
		blk := func() {
			var v T
			_ = init(&v)
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
