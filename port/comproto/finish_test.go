package comproto_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"go.llib.dev/frameless/internal/doubles"

	"go.llib.dev/frameless/port/comproto"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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
		defer comproto.FinishTx(&returnError, tx.Commit, tx.Rollback)
		// do something with in comproto
		return nil
	}

	_ = myMethod
}

func ExampleFinishOnePhaseCommit() {
	var cm comproto.OnePhaseCommitProtocol

	myMethod := func(ctx context.Context) (returnError error) {
		tx, err := cm.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&returnError, cm, tx)
		// do something with in comproto
		return nil
	}

	_ = myMethod
}

func TestFinishTx(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errp = testcase.Let(s, func(t *testcase.T) interface{} {
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
		commitFn  = testcase.Let(s, func(t *testcase.T) interface{} {
			return func() error { return CommitErr }
		})
		commitFnGet = func(t *testcase.T) func() error { return commitFn.Get(t).(func() error) }
		rolledBack  = testcase.LetValue(s, false)
		rollbackFn  = testcase.Let(s, func(t *testcase.T) interface{} {
			return func() error {
				rolledBack.Set(t, true)
				return nil
			}
		})
		rollbackFnGet = func(t *testcase.T) func() error { return rollbackFn.Get(t).(func() error) }
	)

	subject := func(t *testcase.T) {
		comproto.FinishTx(errpGet(t), commitFnGet(t), rollbackFnGet(t))
	}

	s.When(`error pointer is not initialized`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			return nil
		})

		s.Then(`it will panic as this is an invalid use-case for this function`, func(t *testcase.T) {
			t.Must.Panic(func() { subject(t) })
		})
	})

	s.When(`error pointer points to a valid error variable with nil content`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			var err error
			return &err
		})

		s.Then(`it will commit and return the commit error value`, func(t *testcase.T) {
			subject(t)
			assert.Equal(t, CommitErr, *errpGet(t))
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
			assert.True(t, rolledBack.Get(t))
			assert.Equal(t, expectedErr, *errpGet(t))
		})
	})
}

func TestFinishOnePhaseCommit(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errp = testcase.Let(s, func(t *testcase.T) interface{} {
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
		rolledBack    = testcase.LetValue(s, false)
	)
	cpm := testcase.Let(s, func(t *testcase.T) interface{} {
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
		tx = testcase.Let(s, func(t *testcase.T) interface{} {
			ctx := context.Background()
			tx, err := cpmGet(t).BeginTx(ctx)
			t.Must.NoError(err)
			return tx
		})
		txGet = func(t *testcase.T) context.Context { return tx.Get(t).(context.Context) }
	)

	subject := func(t *testcase.T) {
		comproto.FinishOnePhaseCommit(errpGet(t), cpmGet(t), txGet(t))
	}

	s.When(`error pointer is not initialized`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			return nil
		})

		s.Then(`it will panic as this is an invalid use-case for this function`, func(t *testcase.T) {
			assert.Must(t).Panic(func() { subject(t) })
		})
	})

	s.When(`error pointer points to a valid error variable with nil content`, func(s *testcase.Spec) {
		errpLet(s, func(t *testcase.T) *error {
			var err error
			return &err
		})

		s.Then(`it will commit and return the commit error value`, func(t *testcase.T) {
			subject(t)
			assert.Equal(t, CommitTxErr, *errpGet(t))
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
			assert.True(t, rolledBack.Get(t))
			assert.Must(t).ErrorIs(expectedErr, *errpGet(t))
		})
	})
}
