package errorkit_test

import (
	"database/sql"
	"errors"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"runtime"
	"sync"
	"testing"
)

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
			defer errorkit.Finish(&rErr, func() error { return nil })

			return nil
		}()

		assert.NoError(t, got)
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
			t.Must.Nil(act(t))
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
				t.Must.Nil(act(t))
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
				t.Must.Nil(act(t))
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
		actionLet(s, func() error { return nil })

		s.Then(`it will do nothing`, func(t *testcase.T) {
			assert.Must(t).Nil(subject(t))
		})
	})

	s.When(`action returns an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { return expectedErr })

		s.Then(`it will pass the received error through`, func(t *testcase.T) {
			assert.Must(t).Equal(expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			assert.Must(t).Equal(expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			assert.Must(t).Equal(expectedErr, subject(t))
		})
	})

	s.When(`action panics with an non error type`, func(s *testcase.Spec) {
		const msg = `boom`
		actionLet(s, func() error { panic(msg) })

		s.Then(`it will capture the panic value and create an error from it, where message is the panic object is formatted with fmt`, func(t *testcase.T) {
			assert.Must(t).Equal(errors.New("boom"), subject(t))
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
