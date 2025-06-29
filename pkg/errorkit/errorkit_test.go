package errorkit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"go.llib.dev/testcase/sandbox"
)

var rnd = random.New(random.CryptoSeed{})

type ErrT struct{ V any }

func (err ErrT) Error() string { return fmt.Sprintf("%T:%v", err, err.V) }

func ExampleError() {
	var (
		err1 error = errors.New("first error")
		err2 error = errors.New("second error")
		err3 error = nil
	)

	err := errorkit.Merge(err1, err2, err3)
	errors.Is(err, err1) // true
	errors.Is(err, err2) // true
	errors.Is(err, err3) // true
}

func ExampleError_Error() {
	const ErrSomething errorkit.Error = "something is an error"

	_ = ErrSomething
}

func TestError_Error_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	assert.Equal(t, ErrExample.Error(), string(ErrExample))
}

type ErrAsStub struct {
	V string
}

func (err ErrAsStub) Error() string {
	return fmt.Sprintf("ErrAsStub: %s", err.V)
}

func TestError_Wrap_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	t.Run("happy", func(t *testing.T) {
		exp := rnd.Error()
		got := ErrExample.Wrap(exp)
		assert.ErrorIs(t, got, exp)
		assert.ErrorIs(t, got, ErrExample)
		assert.Contain(t, got.Error(), fmt.Sprintf("[%s] %s", ErrExample, exp.Error()))

		t.Run("Is", func(t *testing.T) {
			assert.True(t, errors.Is(got, ErrExample))
			assert.True(t, errors.Is(got, exp))
		})

		t.Run("As", func(t *testing.T) {
			exp := ErrAsStub{V: rnd.String()}
			got := ErrExample.Wrap(exp)
			assert.ErrorIs(t, got, exp)
			assert.ErrorIs(t, got, ErrExample)

			var expected ErrAsStub
			assert.True(t, errors.As(got, &expected))
			assert.Equal(t, exp, expected)
		})
	})
	t.Run("nil", func(t *testing.T) {
		got := ErrExample.Wrap(nil)
		assert.ErrorIs(t, got, ErrExample)
		assert.Equal[error](t, got, ErrExample)
	})
}

func TestError_F_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	t.Run("sprintf", func(t *testing.T) {
		got := ErrExample.F("foo - bar - %s", "baz")
		assert.ErrorIs(t, got, ErrExample)
		assert.Contain(t, got.Error(), "foo - bar - baz")
	})
	t.Run("errorf", func(t *testing.T) {
		exp := rnd.Error()
		got := ErrExample.F("%w", exp)
		assert.ErrorIs(t, got, ErrExample)
		assert.ErrorIs(t, got, exp)
		assert.Contain(t, got.Error(), ErrExample.Error())
	})
}

func TestError_traced(t *testing.T) {
	const ErrBase errorkit.Error = "base error"

	var assertTraced = func(t *testing.T, err error) {
		var traced errorkit.TracedError
		assert.True(t, errors.As(err, &traced))
		assert.NotNil(t, traced.Err)
		assert.NotEmpty(t, traced.Stack)
	}

	assertTraced(t, ErrBase.F("traced"))
	assertTraced(t, ErrBase.Wrap(rnd.Error()))
}

func ExampleFinish_sqlRows() {
	var db *sql.DB

	myExampleFunction := func() (rErr error) {
		rows, err := db.Query("SELECT FROM mytable")
		if err != nil {
			return err
		}
		defer errorkit.Finish(&rErr, rows.Close)

		for rows.Next() {
			if err := rows.Scan(); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	if err := myExampleFunction(); err != nil {
		panic(err.Error())
	}
}

func TestFinish(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("errors are merged from all source", func(t *testing.T) {
		err1 := rnd.Error()
		err2 := rnd.Error()

		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return err1
			})

			return err2
		}()

		assert.ErrorIs(t, err1, got)
		assert.ErrorIs(t, err2, got)
	})

	t.Run("Finish error is returned", func(t *testing.T) {
		exp := rnd.Error()
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return exp
			})

			return nil
		}()

		assert.ErrorIs(t, exp, got)
	})

	t.Run("func return value returned", func(t *testing.T) {
		exp := rnd.Error()
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return nil
			})

			return exp
		}()

		assert.ErrorIs(t, exp, got)
	})

	t.Run("nothing fails, no error returned", func(t *testing.T) {
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return nil
			})

			return nil
		}()

		assert.NoError(t, got)
	})
}

func TestRecover(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		action    = testcase.Var[func() error]{ID: `action`}
		actionLet = func(s *testcase.Spec, fn func() error) { action.Let(s, func(t *testcase.T) func() error { return fn }) }
	)
	subject := func(t *testcase.T) (err error) {
		defer errorkit.Recover(&err)
		return action.Get(t)()
	}

	s.When(`action ends without error`, func(s *testcase.Spec) {
		actionLet(s, func() error {
			return nil
		})

		s.Then(`it will do nothing`, func(t *testcase.T) {
			assert.Must(t).NoError(subject(t))
		})
	})

	s.When(`action returns an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { return expectedErr })

		s.Then(`it will pass the received error through`, func(t *testcase.T) {
			assert.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			assert.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			assert.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an non error type`, func(s *testcase.Spec) {
		const msg = `boom`
		actionLet(s, func() error { panic(msg) })

		s.Then(`it will capture the panic value and create an error from it, where message is the panic object is formatted with fmt`, func(t *testcase.T) {
			assert.Equal(t, errors.New("boom"), subject(t))
		})
	})

	s.When(`action stops the go routine`, func(s *testcase.Spec) {
		actionLet(s, func() error {
			runtime.Goexit()
			return nil
		})

		s.Then(`it will let go exit continues`, func(t *testcase.T) {
			var (
				wg       = &sync.WaitGroup{}
				finished bool
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = subject(t)
				finished = true
			}()
			wg.Wait()
			assert.Must(t).False(finished)
		})
	})
}

func ExampleRecoverWith() {
	defer errorkit.RecoverWith(func(r any) { /* do something with "r" */ })

	/* do something that might panic */
}

func TestRecoverWith(t *testing.T) {
	s := testcase.NewSpec(t)

	act := func(t *testcase.T, blk func(r any), action func()) {
		out := sandbox.Run(func() {
			defer errorkit.RecoverWith(blk)
			action()
		})
		assert.True(t, out.OK, "no panic is expected after RecoverWith")
	}

	s.Test("no panic", func(t *testcase.T) {
		var got any
		act(t, func(r any) { got = r }, func() { /* OK */ })
		assert.Nil(t, got)
	})

	s.Test("panic", func(t *testcase.T) {
		var got any
		var exp = rnd.Error()
		act(t, func(r any) { got = r }, func() { panic(exp) })
		assert.NotNil(t, got)
		assert.Equal[any](t, exp, got)
	})
}

func BenchmarkFinish(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		errorkit.Finish(&err, func() error {
			return nil
		})
	}
}

type (
	ErrType1 struct{}
	ErrType2 struct{ V int }
)

func (err ErrType1) Error() string { return "ErrType1" }
func (err ErrType2) Error() string { return "ErrType2" }

type MyError struct {
	Msg string
}

func (err MyError) Error() string {
	return err.Msg
}

func ExampleAs() {
	var err error // some error to be checked

	if err, ok := errorkit.As[MyError](err); ok {
		fmt.Println(err.Msg)
	}
}

func TestAs(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		expErr := MyError{Msg: rnd.Error().Error()}
		var err error = fmt.Errorf("wrapped: %w", expErr)
		gotErr, ok := errorkit.As[MyError](err)
		assert.True(t, ok)
		assert.Equal(t, gotErr, expErr)
	})
	t.Run("rainy", func(t *testing.T) {
		var err error = fmt.Errorf("wrapped: %w", rnd.Error())
		gotErr, ok := errorkit.As[MyError](err)
		assert.False(t, ok)
		assert.Empty(t, gotErr)
	})
	t.Run("nil", func(t *testing.T) {
		gotErr, ok := errorkit.As[MyError](nil)
		assert.False(t, ok)
		assert.Empty(t, gotErr)
	})
}

func ExampleFinishOnError() {
	// example function
	var _ = func() (rerr error) {
		defer errorkit.FinishOnError(&rerr, func() { /* do something when the */ })

		return errorkit.Error("boom")
	}
}

func TestFinishOnError(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		returnErr = testcase.Let[error](s, nil)
		blockRan  = testcase.LetValue(s, false)
	)
	act := func(t *testcase.T) {
		defer errorkit.FinishOnError(pointer.Of(returnErr.Get(t)), func() { blockRan.Set(t, true) })
	}

	s.When("return error is nil", func(s *testcase.Spec) {
		returnErr.LetValue(s, nil)

		s.Then("it will not execute the block", func(t *testcase.T) {
			act(t)

			assert.False(t, blockRan.Get(t))
		})
	})

	s.When("return was a valid error value", func(s *testcase.Spec) {
		returnErr.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		s.Then("it will execute the block", func(t *testcase.T) {
			act(t)

			assert.True(t, blockRan.Get(t))
		})
	})
}

func ExampleWithContext() {
	err := fmt.Errorf("foo bar baz")
	ctx := context.Background()

	err = errorkit.WithContext(err, ctx)
	_, _ = errorkit.LookupContext(err) // ctx, true
}

func TestWithContext(t *testing.T) {
	s := testcase.NewSpec(t)

	type (
		fooKey struct{}
		oofKey struct{}
	)
	var (
		err = let.Error(s)
		ctx = let.Context(s).Let(s, func(t *testcase.T) context.Context {
			ctx := context.Background()
			ctx = context.WithValue(ctx, fooKey{}, "bar")
			ctx = context.WithValue(ctx, oofKey{}, "rab")
			return ctx
		})
	)
	act := func(t *testcase.T) error {
		return errorkit.WithContext(err.Get(t), ctx.Get(t))
	}

	s.Then("context can be looked up", func(t *testcase.T) {
		_, ok := errorkit.LookupContext(err.Get(t))
		t.Must.False(ok)

		gotCtx, ok := errorkit.LookupContext(act(t))
		t.Must.True(ok)
		t.Must.Equal(ctx.Get(t), gotCtx)
		t.Must.Equal("bar", gotCtx.Value(fooKey{}).(string))
	})

	s.Then(".Error() returns the underlying error's result", func(t *testcase.T) {
		t.Must.Equal(err.Get(t).Error(), act(t).Error())
	})

	s.When("the input error has a typed error", func(s *testcase.Spec) {
		expectedTypedError := testcase.Let(s, func(t *testcase.T) errorkit.UserError {
			return errorkit.UserError{
				Code:    "foo-bar-baz",
				Message: "The foo, bar and the baz",
			}
		})
		err.Let(s, func(t *testcase.T) error {
			return expectedTypedError.Get(t)
		})

		s.Then("the typed error can be looked up with errors.As", func(t *testcase.T) {
			var usrErr errorkit.UserError
			t.Must.True(errors.As(act(t), &usrErr))
			t.Must.Equal(expectedTypedError.Get(t), usrErr)
		})

		s.Then("we can check after the typed error with errors.Is", func(t *testcase.T) {
			t.Must.True(errors.Is(act(t), expectedTypedError.Get(t)))
		})
	})

	s.When("the input error is nil", func(s *testcase.Spec) {
		err.LetValue(s, nil)

		s.Then("the returned error is also nil", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})
	})
}

func ExampleMerge() {
	// creates an error value that combines the input errors.
	err := errorkit.Merge(fmt.Errorf("foo"), fmt.Errorf("bar"), nil)
	_ = err
}

func TestMerge(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errs = testcase.Let[[]error](s, nil)
	)
	act := func(t *testcase.T) error {
		return errorkit.Merge(errs.Get(t)...)
	}

	s.When("no error is supplied", func(s *testcase.Spec) {
		errs.Let(s, func(t *testcase.T) []error {
			return []error{}
		})

		s.Then("it will return with nil", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})
	})

	s.When("an error value is supplied", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		errs.Let(s, func(t *testcase.T) []error {
			return []error{expectedErr.Get(t)}
		})

		s.Then("the exact value is returned", func(t *testcase.T) {
			t.Must.Equal(expectedErr.Get(t), act(t))
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})

		s.And("the error value is a typed error value", func(s *testcase.Spec) {
			expectedErr.LetValue(s, ErrType1{})

			s.Then("the exact value is returned", func(t *testcase.T) {
				t.Must.Equal(expectedErr.Get(t), act(t))
			})

			s.Then("errors.Is will find wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As will find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})

		s.And("but the error value is nil", func(s *testcase.Spec) {
			expectedErr.LetValue(s, nil)

			s.Then("it will return with nil", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})

			s.Then("errors.Is yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})
	})

	s.When("multiple error values are supplied", func(s *testcase.Spec) {
		expectedErr1 := let.Error(s)
		expectedErr2 := let.Error(s)
		expectedErr3 := let.Error(s)

		errs.Let(s, func(t *testcase.T) []error {
			return []error{
				expectedErr1.Get(t),
				expectedErr2.Get(t),
				expectedErr3.Get(t),
			}
		})

		s.Then("retruned value includes all three error value", func(t *testcase.T) {
			err := act(t)
			t.Must.ErrorIs(expectedErr1.Get(t), err)
			t.Must.ErrorIs(expectedErr2.Get(t), err)
			t.Must.ErrorIs(expectedErr2.Get(t), err)
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})

		s.And("the errors has a typed error value", func(s *testcase.Spec) {
			expectedErr2.LetValue(s, ErrType1{})

			s.Then("the named error value is returned", func(t *testcase.T) {
				t.Must.ErrorIs(expectedErr2.Get(t), act(t))
			})

			s.Then("errors.Is can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})

		s.And("the errors has multiple typed error value", func(s *testcase.Spec) {
			expectedErr2.LetValue(s, ErrType1{})
			expectedErr3.Let(s, func(t *testcase.T) error {
				return ErrType2{V: t.Random.Int()}
			})

			s.Then("returned error contains all typed error", func(t *testcase.T) {
				t.Must.ErrorIs(expectedErr2.Get(t), act(t))
				t.Must.ErrorIs(expectedErr3.Get(t), act(t))
			})

			s.Then("errors.Is can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, expectedErr2.Get(t)))
				t.Must.True(errors.Is(err, expectedErr3.Get(t)))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))

				var gotErrWithAs ErrType2
				t.Must.True(errors.As(err, &gotErrWithAs))
				t.Must.NotNil(gotErrWithAs)
				t.Must.Equal(expectedErr3.Get(t), gotErrWithAs)
			})
		})

		s.And("but the error values are nil", func(s *testcase.Spec) {
			expectedErr1.LetValue(s, nil)
			expectedErr2.LetValue(s, nil)
			expectedErr3.LetValue(s, nil)

			s.Then("it will return with nil", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})

			s.Then("errors.Is yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})
	})
}

func ExampleF() {
	const ErrBoom errorkit.Error = "boom!"

	err := errorkit.F("something went wrong: %w", ErrBoom)

	if traced, ok := errorkit.As[errorkit.TracedError](err); ok {
		fmt.Println(traced.Error())
	}

	fmt.Println(err.Error())
}

func TestF(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		format = testcase.LetValue(s, "foo bar baz : %s %d")
		args   = testcase.Let(s, func(t *testcase.T) []any {
			return []any{"foo", 42}
		})
	)
	act := let.Act(func(t *testcase.T) error {
		return errorkit.F(format.Get(t), args.Get(t)...)
	})

	s.Then("a non-nil, formatted error value is returned", func(t *testcase.T) {
		err := act(t)

		assert.Error(t, err)
		message := fmt.Sprintf(format.Get(t), args.Get(t)...)
		assert.Contain(t, err.Error(), message)
	})

	s.Then("the error has trace", func(t *testcase.T) {
		err := act(t)

		var trace errorkit.TracedError
		assert.True(t, errors.As(err, &trace))
		assert.NotEmpty(t, trace)
		assert.Error(t, trace.Err)
	})

	s.When("format wraps another error", func(s *testcase.Spec) {
		expErr := let.Error(s)
		format.LetValue(s, "error:%w")
		args.Let(s, func(t *testcase.T) []any { return []any{expErr.Get(t)} })

		s.Then("then the passed error's content is part of the result error's content", func(t *testcase.T) {
			err := act(t)
			assert.Error(t, err)
			assert.Contain(t, err.Error(), fmt.Sprintf("error:%s", expErr.Get(t).Error()))
		})

		s.Then("the result error wraps the passed error", func(t *testcase.T) {
			err := act(t)
			assert.Error(t, err)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})
}
