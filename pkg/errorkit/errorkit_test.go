package errorkit_test

import (
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

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
