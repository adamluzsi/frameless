package lazyload_test

import (
	"github.com/adamluzsi/frameless/pkg/lazyload"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestMake(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		initWasCalled = testcase.LetValue(s, false)
		initFunc      = testcase.Let(s, func(t *testcase.T) func() int {
			return func() int {
				initWasCalled.Set(t, true)
				return t.Random.Int()
			}
		})
	)
	act := func(t *testcase.T) func() int {
		return lazyload.Make(initFunc.Get(t))
	}

	s.Test(`assuming that init block always yield a different value`, func(t *testcase.T) {
		t.Must.NotEqual(initFunc.Get(t)(), initFunc.Get(t)())
	})

	s.Then(`calling lazy loaded value multiple times return the same result`, func(t *testcase.T) {
		llv := act(t)
		t.Must.Equal(llv(), llv())
	})

	s.Then(`before calling lazy loaded value, init block is not used`, func(t *testcase.T) {
		assert.Must(t).False(initWasCalled.Get(t))
		act(t)()
		assert.Must(t).True(initWasCalled.Get(t))
	})

	s.Test(`safe for concurrent use`, func(t *testcase.T) {
		llv := act(t)
		testcase.Race(
			func() { llv() },
			func() { llv() },
		)
	})
}

func TestVar(t *testing.T) {
	s := testcase.NewSpec(t)

	lazyVar := testcase.Let(s, func(t *testcase.T) *lazyload.Var[int] {
		return &lazyload.Var[int]{}
	})

	s.Describe(".Get", func(s *testcase.Spec) {
		var (
			inits = testcase.LetValue[[]func() int](s, nil)
		)
		act := func(t *testcase.T) int {
			return lazyVar.Get(t).Get(inits.Get(t)...)
		}

		s.Then("by default it returns the zero value", func(t *testcase.T) {
			t.Must.Equal(int(0), act(t))
		})

		s.When(".Init block set", func(s *testcase.Spec) {
			expValue := let.Int(s)

			s.Before(func(t *testcase.T) {
				v := expValue.Get(t)
				lazyVar.Get(t).Init = func() int {
					defer func() { v++ }() // to represent a changing init result
					return v
				}
			})

			s.Then(".Init block used to initialize the value", func(t *testcase.T) {
				t.Must.Equal(expValue.Get(t), act(t))
			})

			s.Then(".Init value is cached", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					t.Must.Equal(expValue.Get(t), act(t))
				})
			})
		})

		s.When("init block supplied through the Get variable", func(s *testcase.Spec) {
			expValue := let.Int(s)

			inits.Let(s, func(t *testcase.T) []func() int {
				v := expValue.Get(t)
				return []func() int{
					func() int {
						defer func() { v++ }()
						return v
					},
				}
			})

			s.Then("init block used to initialize the value", func(t *testcase.T) {
				t.Must.Equal(expValue.Get(t), act(t))
			})

			s.Then("init value is cached", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					t.Must.Equal(expValue.Get(t), act(t))
				})
			})

			s.Then("Var.Init remains uninitialised", func(t *testcase.T) {
				act(t)

				t.Must.Nil(lazyVar.Get(t).Init)
			})

			s.Then("calling .Get without init block beforehand don't trigger the Initialisation with Get+init", func(t *testcase.T) {
				t.Must.Empty(lazyVar.Get(t).Get())
				t.Must.Equal(expValue.Get(t), act(t))
				t.Must.Equal(expValue.Get(t), lazyVar.Get(t).Get())
			})
		})
	})

	s.Describe(".Set", func(s *testcase.Spec) {
		var (
			v = let.Int(s)
		)
		act := func(t *testcase.T) {
			lazyVar.Get(t).Set(v.Get(t))
		}

		s.Then("it will set the value to the Var", func(t *testcase.T) {
			act(t)

			t.Must.Equal(v.Get(t), lazyVar.Get(t).Get())
		})

		s.When(".Init is supplied", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				lazyVar.Get(t).Init = func() int {
					return t.Random.Int()
				}
			})

			s.Then("set will overwrite the initialization process", func(t *testcase.T) {
				act(t)

				t.Must.Equal(v.Get(t), lazyVar.Get(t).Get())
			})
		})
	})

	s.Describe(".Reset", func(s *testcase.Spec) {
		act := func(t *testcase.T) {
			lazyVar.Get(t).Reset()
		}

		s.Before(func(t *testcase.T) {
			t.Log("given a value is already loaded in the Var")
			lazyVar.Get(t).Get(t.Random.Int)
		})

		s.Then("then it will reset the state", func(t *testcase.T) {
			act(t)
			t.Must.Equal(0, lazyVar.Get(t).Get())
			t.Must.Equal(42, lazyVar.Get(t).Get(func() int { return 42 }))
		})
	})

	s.Test("race", func(t *testcase.T) {
		lv := lazyVar.Get(t)
		get := func() { _ = lv.Get(func() int { return 42 }) }
		set := func() { lv.Set(42) }
		reset := func() { lv.Reset() }
		testcase.Race(get, get, get, set, set, set, reset, reset, reset)
	})
}

func TestMake_withPointerType(t *testing.T) {
	v := lazyload.Make(func() *int {
		var n int = 42
		return &n
	})
	ptr := v()
	expected := random.New(random.CryptoSeed{}).Int()
	*(v()) = expected
	assert.Equal(t, expected, *ptr)
}

func Benchmark_rangeVsAccessByIndex(b *testing.B) {
	var (
		init func()
		_    = init
		v    = []int{1, 2, 3}
	)
	b.Run("index", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var x int
			if init == nil && 0 < len(v) {
				x = v[0]
			}
			_ = x
		}
	})
	b.Run("range", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var x int
			if init == nil {
				for _, e := range v {
					x = e
					break
				}
			}
			_ = x
		}
	})
}
