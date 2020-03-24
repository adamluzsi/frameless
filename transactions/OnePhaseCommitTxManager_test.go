package transactions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/transactions"
)

func TestOnePhaseCommitTxManager(t *testing.T) {
	var s = testcase.NewSpec(t)
	s.Parallel()

	s.Let(`*transactions.OnePhaseCommitTxManager`, func(t *testcase.T) interface{} {
		return transactions.NewOnePhaseCommitTxManager()
	})
	var manager = func(t *testcase.T) *transactions.OnePhaseCommitTxManager {
		return t.I(`*transactions.OnePhaseCommitTxManager`).(*transactions.OnePhaseCommitTxManager)
	}

	var ctx = func(t *testcase.T) context.Context {
		return t.I(`ctx`).(context.Context)
	}
	var ctxAccessor = func(t *testcase.T) transactions.ContextAccessor {
		return t.I(`ContextAccessor`).(transactions.ContextAccessor)
	}
	s.Let(`BeginTxErr`, func(t *testcase.T) interface{} { return nil })
	s.Let(`TxErr`, func(t *testcase.T) interface{} { return nil })
	s.Let(`ContextAccessor`, func(t *testcase.T) interface{} {
		var beginTxErr, _ = t.I(`BeginTxErr`).(error)
		var txErr, _ = t.I(`TxErr`).(error)
		return manager(t).RegisterAdapter(TxAdapter{BeginTxErr: beginTxErr, TxErr: txErr})
	})
	s.Before(func(t *testcase.T) {
		t.Log(`given the adapter is registered and because that we have a ctx accessor`)
		ctxAccessor(t) // early load
	})

	s.Let(`ctx`, func(t *testcase.T) interface{} { return context.Background() })

	s.When(`ctx has no tx management enabled`, func(s *testcase.Spec) {

		s.And(`an error occurs during tx creation`, func(s *testcase.Spec) {
			var beginTxErrMsg = fixtures.Random.String()
			s.Let(`BeginTxErr`, func(t *testcase.T) interface{} { return errors.New(beginTxErrMsg) })

			s.Then(`during the tx fetch from the context, the error will be returned`, func(t *testcase.T) {
				_, _, err := ctxAccessor(t).FromContext(ctx(t))
				require.EqualError(t, err, beginTxErrMsg)
			})
		})

		s.And(`an error occurs with the tx commit phase`, func(s *testcase.Spec) {
			var txErrMsg = fixtures.Random.String()
			s.Let(`TxErr`, func(t *testcase.T) interface{} { return errors.New(txErrMsg) })

			s.Then(`during the  tx fetch from the context, the error will be returned`, func(t *testcase.T) {
				_, sf, err := ctxAccessor(t).FromContext(ctx(t))
				require.Nil(t, err)
				require.EqualError(t, sf.Done(), txErrMsg)
			})
		})

		s.Then(`accessing the tx from the context results different transaction object`, func(t *testcase.T) {
			// do some action with a tx resource
			tx1Interface, sf, err := ctxAccessor(t).FromContext(ctx(t))
			tx1 := tx1Interface.(*Tx)
			require.Nil(t, err)
			require.Nil(t, sf.Done())

			// accessing the same ctx with the same ctx accessor returns the same tx
			tx2Interface, sf, err := ctxAccessor(t).FromContext(ctx(t))
			tx2 := tx2Interface.(*Tx)
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.NotEqual(t, tx1, tx2)
		})

		s.Then(`calling Done on step finalizer will commit, since no transaction management expected`, func(t *testcase.T) {
			t.Log(`the default expected behavior with null object pattern is that calling Done() not just finalize the current scope but the tx as well with a commit`)
			txi, sf, err := ctxAccessor(t).FromContext(ctx(t))
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.True(t, txi.(*Tx).committed)
			require.False(t, txi.(*Tx).rolledback)
		})

	})

	s.When(`ctx has tx management enabled`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			ctx, handler := manager(t).ContextWithTransactionManagement(t.I(`ctx`).(context.Context))
			t.Let(`handler`, handler)
			t.Let(`ctx`, ctx)
		})

		s.And(`an error occurs during tx creation`, func(s *testcase.Spec) {
			var beginTxErrMsg = fixtures.Random.String()
			s.Let(`BeginTxErr`, func(t *testcase.T) interface{} { return errors.New(beginTxErrMsg) })

			s.Then(`during the tx fetch from the context, the error will be returned`, func(t *testcase.T) {
				_, _, err := ctxAccessor(t).FromContext(ctx(t))
				require.EqualError(t, err, beginTxErrMsg)
			})
		})

		s.And(`an error occurs with the tx commit phase`, func(s *testcase.Spec) {
			var txErrMsg = fixtures.Random.String()
			s.Let(`TxErr`, func(t *testcase.T) interface{} { return errors.New(txErrMsg) })

			s.Then(`during the  tx fetch from the context, the error will be returned`, func(t *testcase.T) {
				_, sf, err := ctxAccessor(t).FromContext(ctx(t))
				require.Nil(t, err)
				require.Nil(t, sf.Done())

				var handler = t.I(`handler`).(transactions.Handler)
				require.EqualError(t, handler.Commit(), txErrMsg)
			})
		})

		s.Then(`accessing the tx from the context results in the same transaction object`, func(t *testcase.T) {
			// do some action with a tx resource
			tx1Interface, sf, err := ctxAccessor(t).FromContext(ctx(t))
			tx1 := tx1Interface.(*Tx)
			require.Nil(t, err)
			require.Nil(t, sf.Done())

			// accessing the same ctx with the same ctx accessor returns the same tx
			tx2Interface, sf, err := ctxAccessor(t).FromContext(ctx(t))
			tx2 := tx2Interface.(*Tx)
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.Equal(t, tx1, tx2)
		})

		s.Then(`calling Done on step finalizer will not commit or rollback, since transaction management expected by the tx manager`, func(t *testcase.T) {
			t.Log(`this is convenient since the user of the ctx accessor don't need knowledge about that the current functionality is executed as a single unit or as part of many'`)
			txi, sf, err := ctxAccessor(t).FromContext(ctx(t))
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.False(t, txi.(*Tx).committed)
			require.False(t, txi.(*Tx).rolledback)
		})

		s.Then(`calling commit on the tx manager context handler will commit`, func(t *testcase.T) {
			txi, sf, err := ctxAccessor(t).FromContext(ctx(t))
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.False(t, txi.(*Tx).committed)
			require.False(t, txi.(*Tx).rolledback)
			require.Nil(t, t.I(`handler`).(transactions.Handler).Commit())
			require.True(t, txi.(*Tx).committed)
			require.False(t, txi.(*Tx).rolledback)
		})

		s.Then(`calling rollback on the tx manager context handler will rollback`, func(t *testcase.T) {
			txi, sf, err := ctxAccessor(t).FromContext(ctx(t))
			require.Nil(t, err)
			require.Nil(t, sf.Done())
			require.False(t, txi.(*Tx).committed)
			require.False(t, txi.(*Tx).rolledback)
			require.Nil(t, t.I(`handler`).(transactions.Handler).Rollback())
			require.False(t, txi.(*Tx).committed)
			require.True(t, txi.(*Tx).rolledback)
		})
	})
}

type Tx struct {
	Err        error
	committed  bool
	rolledback bool
	id         string
}

type TxAdapter struct {
	BeginTxErr error
	TxErr      error
}

func (t TxAdapter) BeginTx(context.Context) (ptr interface{}, err error) {
	return &Tx{id: fixtures.Random.String(), Err: t.TxErr}, t.BeginTxErr
}

func (t TxAdapter) Commit(ptr interface{}) error {
	ptr.(*Tx).committed = true
	return ptr.(*Tx).Err
}

func (t TxAdapter) Rollback(ptr interface{}) error {
	ptr.(*Tx).rolledback = true
	return ptr.(*Tx).Err
}
