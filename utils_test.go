package frameless_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func ExampleFinishTx() {
	db, err := sql.Open(`fake`, `DSN`)
	if err != nil {
		panic(err)
	}

	myMethod := func(ctx context.Context) (returnError error) {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer frameless.FinishTx(&returnError, tx.Commit, tx.Rollback)
		// do something with in tx
		return nil
	}

	_ = myMethod
}
func ExampleFinishOnePhaseCommit() {
	var cm frameless.OnePhaseCommitProtocol

	myMethod := func(ctx context.Context) (returnError error) {
		tx, err := cm.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer frameless.FinishOnePhaseCommit(&returnError, cm, tx)
		// do something with in tx
		return nil
	}

	_ = myMethod
}

func TestRecover(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		action    = testcase.Var{Name: `action`}
		actionLet = func(s *testcase.Spec, fn func() error) { action.Let(s, func(t *testcase.T) interface{} { return fn }) }
		actionGet = func(t *testcase.T) func() error { return action.Get(t).(func() error) }
	)
	subject := func(t *testcase.T) (err error) {
		defer frameless.Recover(&err)
		return actionGet(t)()
	}

	s.When(`action ends without error`, func(s *testcase.Spec) {
		actionLet(s, func() error { return nil })

		s.Then(`it will do nothing`, func(t *testcase.T) {
			require.Nil(t, subject(t))
		})
	})

	s.When(`action returns an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { return expectedErr })

		s.Then(`it will pass the received error through`, func(t *testcase.T) {
			require.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			require.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		actionLet(s, func() error { panic(expectedErr) })

		s.Then(`it will capture the error from panic and returns with it`, func(t *testcase.T) {
			require.Equal(t, expectedErr, subject(t))
		})
	})

	s.When(`action panics with an non error type`, func(s *testcase.Spec) {
		const msg = `boom`
		actionLet(s, func() error { panic(msg) })

		s.Then(`it will capture the panic value and create an error from it, where message is the panic object is formatted with fmt`, func(t *testcase.T) {
			require.Equal(t, errors.New("boom"), subject(t))
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
			require.False(t, finished)
		})
	})
}

func TestFinishTx(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errp = s.Let(`error pointer`, func(t *testcase.T) interface{} {
			var err error
			return &err
		})
		errpGet = func(t *testcase.T) *error {
			ptr, _ := errp.Get(t).(*error)
			return ptr
		}
		errpLet = func(s *testcase.Spec, init func(t *testcase.T) *error) {
			errp.Let(s, func(t *testcase.T) interface{} { return init(t) })
		}
	)

	var (
		CommitErr = fmt.Errorf(`CommitErr`)
		commitFn  = s.Let(`commit fn`, func(t *testcase.T) interface{} {
			return func() error { return CommitErr }
		})
		commitFnGet = func(t *testcase.T) func() error { return commitFn.Get(t).(func() error) }
		rolledBack  = s.LetValue(`rolled back`, false)
		rollbackFn  = s.Let(`rollback fn`, func(t *testcase.T) interface{} {
			return func() error {
				rolledBack.Set(t, true)
				return nil
			}
		})
		rollbackFnGet = func(t *testcase.T) func() error { return rollbackFn.Get(t).(func() error) }
	)

	subject := func(t *testcase.T) {
		frameless.FinishTx(errpGet(t), commitFnGet(t), rollbackFnGet(t))
	}

	s.When(`error pointer is not initialized`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			return nil
		})

		s.Then(`it will panic as this is an invalid use-case for this function`, func(t *testcase.T) {
			require.Panics(t, func() { subject(t) })
		})
	})

	s.When(`error pointer points to a valid error variable with nil content`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			var err error
			return &err
		})

		s.Then(`it will commit and return the commit error value`, func(t *testcase.T) {
			subject(t)
			require.Equal(t, CommitErr, *errpGet(t))
		})
	})

	s.When(`error pointer points to a valid error variable with concrete value`, func(s *testcase.Spec) {
		expectedErr := fmt.Errorf("boom")
		errpLet(s, func(t *testcase.T) *error {
			err := expectedErr
			return &err
		})

		s.Then(`it will rollback and keep error value in ptr as is to not obscure root cause`, func(t *testcase.T) {
			subject(t)
			require.True(t, rolledBack.Get(t).(bool))
			require.Equal(t, expectedErr, *errpGet(t))
		})
	})
}

func TestFinishOnePhaseCommit(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errp = s.Let(`error pointer`, func(t *testcase.T) interface{} {
			var err error
			return &err
		})
		errpGet = func(t *testcase.T) *error {
			ptr, _ := errp.Get(t).(*error)
			return ptr
		}
		errpLet = func(s *testcase.Spec, init func(t *testcase.T) *error) {
			errp.Let(s, func(t *testcase.T) interface{} { return init(t) })
		}
	)

	var (
		CommitTxErr   = fmt.Errorf(`CommitTxErr`)
		RollbackTxErr = fmt.Errorf(`RollbackTxErr`)
		rolledBack    = s.LetValue(`rolled back`, false)
		rolledBackGet = func(t *testcase.T) bool { return rolledBack.Get(t).(bool) }
	)
	cpm := s.Let(`commit manager`, func(t *testcase.T) interface{} {
		return &doubles.StubOnePhaseCommitProtocol{
			OnePhaseCommitProtocol: nil,
			BeginTxFunc: func(ctx context.Context) (context.Context, error) {
				return ctx, nil
			},
			CommitTxFunc: func(ctx context.Context) error {
				return CommitTxErr
			},
			RollbackTxFunc: func(ctx context.Context) error {
				rolledBack.Set(t, true)
				return RollbackTxErr
			},
		}
	})
	cpmGet := func(t *testcase.T) *doubles.StubOnePhaseCommitProtocol {
		return cpm.Get(t).(*doubles.StubOnePhaseCommitProtocol)
	}

	var (
		tx = s.Let(`context.Context with transaction`, func(t *testcase.T) interface{} {
			ctx := context.Background()
			tx, err := cpmGet(t).BeginTx(ctx)
			require.NoError(t, err)
			return tx
		})
		txGet = func(t *testcase.T) context.Context { return tx.Get(t).(context.Context) }
	)

	subject := func(t *testcase.T) {
		frameless.FinishOnePhaseCommit(errpGet(t), cpmGet(t), txGet(t))
	}

	s.When(`error pointer is not initialized`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			return nil
		})

		s.Then(`it will panic as this is an invalid use-case for this function`, func(t *testcase.T) {
			require.Panics(t, func() { subject(t) })
		})
	})

	s.When(`error pointer points to a valid error variable with nil content`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			var err error
			return &err
		})

		s.Then(`it will commit and return the commit error value`, func(t *testcase.T) {
			subject(t)
			require.Equal(t, CommitTxErr, *errpGet(t))
		})
	})

	s.When(`error pointer points to a valid error variable with concrete value`, func(s *testcase.Spec) {
		expectedErr := fmt.Errorf("boom")
		errpLet(s, func(t *testcase.T) *error {
			err := expectedErr
			return &err
		})

		s.Then(`it will rollback and keep error value in ptr as is to not obscure root cause`, func(t *testcase.T) {
			subject(t)
			require.True(t, rolledBackGet(t))
			require.Equal(t, expectedErr, *errpGet(t))
		})
	})
}
