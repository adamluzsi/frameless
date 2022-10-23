package lazyloading_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/pkg/errorutil"

	"github.com/adamluzsi/frameless/pkg/lazyloading"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func ExampleVar_Value() {
	var v1 = lazyloading.Var[int]{Init: func() (int, error) {
		return 42, nil
	}}

	_, _ = v1.Value() // returns with the result
}

func ExampleVar_Do() {
	var v1 lazyloading.Var[int]

	_ = v1.Do(func() int {
		return 42
	})
}

func ExampleVar_DoErr() {
	var v1 lazyloading.Var[int]

	_, err := v1.DoErr(func() (int, error) {
		return 0, fmt.Errorf("boom")
	})

	fmt.Println(err)
}

func TestVar(t *testing.T) {
	s := testcase.NewSpec(t)

	lazyVar := testcase.Let(s, func(t *testcase.T) *lazyloading.Var[int] {
		return &lazyloading.Var[int]{}
	})
	takeValue := func(v int, err error) int {
		if err != nil {
			panic(err)
		}
		return v
	}

	s.Describe(`.Init + .Value()`, func(s *testcase.Spec) {
		counter := testcase.LetValue[int](s, int(0))
		blk := testcase.Let(s, func(t *testcase.T) func() int {
			return func() int { return t.Random.Int() }
		})
		subject := func(t *testcase.T) {
			lazyVar.Get(t).Init = func() (int, error) {
				counter.Set(t, counter.Get(t)+1)
				return blk.Get(t)(), nil
			}
		}

		s.Then(`the returned lazyloading.Var can return value with .Value`, func(t *testcase.T) {
			subject(t)
			v, err := lazyVar.Get(t).Value()
			assert.Must(t).Nil(err)
			assert.Must(t).NotEmpty(v)
			assert.Must(t).Equal(takeValue(lazyVar.Get(t).Value()), takeValue(lazyVar.Get(t).Value()))
		})

		s.Then(`value returned from the .Init block will be the one set as value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() int { return expected })
			subject(t)
			assert.Must(t).Equal(expected, takeValue(lazyVar.Get(t).Value()))
		})

		s.Then(`calling Value multiple times won't trigger .Init block multiple times`, func(t *testcase.T) {
			subject(t)
			for i := 0; i < 42; i++ {
				_, _ = lazyVar.Get(t).Value()
			}
			assert.Must(t).Equal(1, counter.Get(t))
		})

		s.Test(`if .Value() is called before .Init(), then it will panic`, func(t *testcase.T) {
			assert.Must(t).Panic(func() { _, _ = lazyVar.Get(t).Value() })
		})
	})

	s.Describe(`.Do`, func(s *testcase.Spec) {
		counter := testcase.LetValue[int](s, int(0))
		blk := testcase.Let(s, func(t *testcase.T) func() int {
			return func() int { return t.Random.Int() }
		})
		subject := func(t *testcase.T) interface{} {
			return lazyVar.Get(t).Do(func() int {
				counter.Set(t, counter.Get(t)+1)
				return blk.Get(t)()
			})
		}

		s.Then(`returns the value immediately`, func(t *testcase.T) {
			assert.Must(t).NotEmpty(subject(t))

			assert.Must(t).Equal(subject(t), subject(t))
		})

		s.Then(`calling .Do more than once with a block makes no difference, and only the initial one is used`, func(t *testcase.T) {
			llv := lazyVar.Get(t)
			subject(t)
			assert.Must(t).NotPanic(func() { llv.Do(func() int { panic("BOOM!") }) })
		})

		s.Then(`value returned from the init block will be the one returned from the value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() int { return expected })
			assert.Must(t).Equal(expected, subject(t))
		})

		s.Then(`calling Value multiple times won't call init block multiple times`, func(t *testcase.T) {
			for i := 0; i < 42; i++ {
				subject(t)
			}
			assert.Must(t).Equal(1, counter.Get(t))
		})

		s.When(`Init block can return error`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`given we incorrectly set .Init with a logic that can fail`)

				// we hack the system by setting constructor block with .DoErr
				_, _ = lazyVar.Get(t).DoErr(func() (int, error) {
					return 0, fmt.Errorf("boom")
				})
			})

			s.Then(`.Do will panic as this is an invalid use of .Var.Do`, func(t *testcase.T) {
				t.Log(`then .Do will panic when it see the error`)
				assert.Must(t).Panic(func() { subject(t) })
			})
		})
	})

	s.Describe(`.DoErr`, func(s *testcase.Spec) {
		counter := testcase.LetValue[int](s, int(0))
		blk := testcase.Let(s, func(t *testcase.T) func() (int, error) {
			return func() (int, error) { return t.Random.Int(), nil }
		})
		subject := func(t *testcase.T) (int, error) {
			return lazyVar.Get(t).DoErr(func() (int, error) {
				counter.Set(t, counter.Get(t)+1)
				return blk.Get(t)()
			})
		}

		s.Then(`returns the value immediately`, func(t *testcase.T) {
			v, err := subject(t)
			assert.Must(t).Nil(err)
			assert.Must(t).NotEmpty(v)
			assert.Must(t).Equal(takeValue(subject(t)), takeValue(subject(t)))
		})

		s.Then(`calling .Do more than once with a block makes no difference, and only the initial one is used`, func(t *testcase.T) {
			llv := lazyVar.Get(t)
			_, _ = subject(t)
			assert.Must(t).NotPanic(func() { _, _ = llv.DoErr(func() (int, error) { panic("BOOM!") }) })
		})

		s.Then(`value returned from the init block will be the one returned from the value`, func(t *testcase.T) {
			expected := t.Random.Int()
			blk.Set(t, func() (int, error) { return expected, nil })
			assert.Must(t).Equal(expected, takeValue(subject(t)))
		})

		s.Then(`calling Value multiple times won't call init block multiple times`, func(t *testcase.T) {
			for i := 0; i < 42; i++ {
				_, _ = subject(t)
			}
			assert.Must(t).Equal(1, counter.Get(t))
		})

		s.When(`block yields an error`, func(s *testcase.Spec) {
			const expectedErr errorutil.Error = "boom"
			blk.Let(s, func(t *testcase.T) func() (int, error) {
				return func() (int, error) {
					return 0, expectedErr
				}
			})

			s.Then(`error is returned`, func(t *testcase.T) {
				_, err := subject(t)
				assert.Must(t).Equal(expectedErr, err)
			})
		})
	})
}
