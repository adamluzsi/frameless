package lazyloading_test

import (
	"testing"

	"github.com/adamluzsi/frameless/lazyloading"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func ExampleVar_Value() {
	var v1 = lazyloading.Var{Init: func() interface{} {
		return 42
	}}

	v1.Value() // returns with the result
}

func ExampleVar_Do() {
	var v1 lazyloading.Var

	_ = v1.Do(func() interface{} {
		return 42
	}).(int) // returns with the result
}

func TestVar(t *testing.T) {
	s := testcase.NewSpec(t)

	varv := s.Let(`#Var`, func(t *testcase.T) interface{} {
		return &lazyloading.Var{}
	})
	varvGet := func(t *testcase.T) *lazyloading.Var {
		return varv.Get(t).(*lazyloading.Var)
	}

	s.Describe(`.Init + .Value()`, func(s *testcase.Spec) {
		counter := s.LetValue(`counter`, int(0))
		blk := s.Let(`block`, func(t *testcase.T) interface{} {
			return func() interface{} { return t.Random.Int() }
		})
		subject := func(t *testcase.T) {
			varvGet(t).Init = func() interface{} {
				counter.Set(t, counter.Get(t).(int)+1)
				return blk.Get(t).(func() interface{})()
			}
		}

		s.Then(`the returned lazyloading.Var can return value with .Value`, func(t *testcase.T) {
			subject(t)
			require.NotZero(t, varvGet(t).Value())
			require.Equal(t, varvGet(t).Value(), varvGet(t).Value())
		})

		s.Then(`value returned from the .Init block will be the one set as value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() interface{} { return expected })
			subject(t)
			require.Equal(t, expected, varvGet(t).Value().(int))
		})

		s.Then(`calling Value multiple times won't trigger .Init block multiple times`, func(t *testcase.T) {
			subject(t)
			for i := 0; i < 42; i++ {
				varvGet(t).Value()
			}
			require.Equal(t, 1, counter.Get(t).(int))
		})

		s.Test(`if .Value() is called before .Init(), then it will panic`, func(t *testcase.T) {
			require.Panics(t, func() { varvGet(t).Value() })
		})
	})

	s.Describe(`.Do`, func(s *testcase.Spec) {
		counter := s.LetValue(`counter`, int(0))
		blk := s.Let(`block`, func(t *testcase.T) interface{} {
			return func() interface{} { return t.Random.Int() }
		})
		subject := func(t *testcase.T) interface{} {
			return varvGet(t).Do(func() interface{} {
				counter.Set(t, counter.Get(t).(int)+1)
				return blk.Get(t).(func() interface{})()
			})
		}

		s.Then(`returns the value immediately`, func(t *testcase.T) {
			require.NotZero(t, subject(t))

			require.Equal(t, subject(t), subject(t))
		})

		s.Then(`calling .Do more than once with a block makes no difference, and only the initial one is used`, func(t *testcase.T) {
			llv := varvGet(t)
			subject(t)
			require.NotPanics(t, func() { llv.Do(func() interface{} { panic("BOOM!") }) })
		})

		s.Then(`value returned from the init block will be the one returned from the value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() interface{} { return expected })
			require.Equal(t, expected, subject(t).(int))
		})

		s.Then(`calling Value multiple times won't call init block multiple times`, func(t *testcase.T) {
			for i := 0; i < 42; i++ {
				subject(t)
			}
			require.Equal(t, 1, counter.Get(t).(int))
		})
	})
}
