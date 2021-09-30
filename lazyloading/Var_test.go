package lazyloading_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/lazyloading"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func ExampleVar_Value() {
	var v1 = lazyloading.Var{Init: func() (interface{}, error) {
		return 42, nil
	}}

	_, _ = v1.Value() // returns with the result
}

func ExampleVar_Do() {
	var v1 lazyloading.Var

	_ = v1.Do(func() interface{} {
		return 42
	}).(int) // returns with the result
}

func ExampleVar_DoErr() {
	var v1 lazyloading.Var

	_, err := v1.DoErr(func() (interface{}, error) {
		return 0, fmt.Errorf("boom")
	})

	fmt.Println(err)
}

func TestVar(t *testing.T) {
	s := testcase.NewSpec(t)

	varv := s.Let(`#Var`, func(t *testcase.T) interface{} {
		return &lazyloading.Var{}
	})
	varvGet := func(t *testcase.T) *lazyloading.Var {
		return varv.Get(t).(*lazyloading.Var)
	}

	takeValue := func(v interface{}, err error) interface{} {
		if err != nil {
			panic(err)
		}
		return v
	}

	s.Describe(`.Init + .Value()`, func(s *testcase.Spec) {
		counter := s.LetValue(`counter`, int(0))
		blk := s.Let(`block`, func(t *testcase.T) interface{} {
			return func() interface{} { return t.Random.Int() }
		})
		subject := func(t *testcase.T) {
			varvGet(t).Init = func() (interface{}, error) {
				counter.Set(t, counter.Get(t).(int)+1)
				return blk.Get(t).(func() interface{})(), nil
			}
		}

		s.Then(`the returned lazyloading.Var can return value with .Value`, func(t *testcase.T) {
			subject(t)
			v, err := varvGet(t).Value()
			require.NoError(t, err)
			require.NotZero(t, v)
			require.Equal(t, takeValue(varvGet(t).Value()), takeValue(varvGet(t).Value()))
		})

		s.Then(`value returned from the .Init block will be the one set as value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() interface{} { return expected })
			subject(t)
			require.Equal(t, expected, takeValue(varvGet(t).Value()).(int))
		})

		s.Then(`calling Value multiple times won't trigger .Init block multiple times`, func(t *testcase.T) {
			subject(t)
			for i := 0; i < 42; i++ {
				_, _ = varvGet(t).Value()
			}
			require.Equal(t, 1, counter.Get(t).(int))
		})

		s.Test(`if .Value() is called before .Init(), then it will panic`, func(t *testcase.T) {
			require.Panics(t, func() { _, _ = varvGet(t).Value() })
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

		s.When(`Init block can return error`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`given we incorrectly set .Init with a logic that can fail`)

				// we hack the system by setting constructor block with .DoErr
				_, _ = varvGet(t).DoErr(func() (interface{}, error) {
					return 0, fmt.Errorf("boom")
				})
			})

			s.Then(`.Do will panic as this is an invalid use of .Var.Do`, func(t *testcase.T) {
				t.Log(`then .Do will panic when it see the error`)
				require.Panics(t, func() { subject(t) })
			})
		})
	})

	s.Describe(`.DoErr`, func(s *testcase.Spec) {
		counter := s.LetValue(`counter`, int(0))
		blk := s.Let(`block`, func(t *testcase.T) interface{} {
			return func() (interface{}, error) { return t.Random.Int(), nil }
		})
		subject := func(t *testcase.T) (interface{}, error) {
			return varvGet(t).DoErr(func() (interface{}, error) {
				counter.Set(t, counter.Get(t).(int)+1)
				return blk.Get(t).(func() (interface{}, error))()
			})
		}

		s.Then(`returns the value immediately`, func(t *testcase.T) {
			v, err := subject(t)
			require.NoError(t, err)
			require.NotZero(t, v)
			require.Equal(t, takeValue(subject(t)), takeValue(subject(t)))
		})

		s.Then(`calling .Do more than once with a block makes no difference, and only the initial one is used`, func(t *testcase.T) {
			llv := varvGet(t)
			_, _ = subject(t)
			require.NotPanics(t, func() { _, _ = llv.DoErr(func() (interface{}, error) { panic("BOOM!") }) })
		})

		s.Then(`value returned from the init block will be the one returned from the value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() (interface{}, error) { return expected, nil })
			require.Equal(t, expected, takeValue(subject(t)).(int))
		})

		s.Then(`calling Value multiple times won't call init block multiple times`, func(t *testcase.T) {
			for i := 0; i < 42; i++ {
				_, _ = subject(t)
			}
			require.Equal(t, 1, counter.Get(t).(int))
		})

		s.When(`block yields an error`, func(s *testcase.Spec) {
			const expectedErr frameless.Error = "boom"
			blk.Let(s, func(t *testcase.T) interface{} {
				return func() (interface{}, error) {
					return 0, expectedErr
				}
			})

			s.Then(`error is returned`, func(t *testcase.T) {
				_, err := subject(t)
				require.Equal(t, expectedErr, err)
			})
		})
	})
}
