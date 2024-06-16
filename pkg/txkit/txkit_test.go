package txkit_test

import (
	"context"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/ports/comproto/comprotocontracts"
	"go.llib.dev/testcase"
)

func Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.HasSideEffect()

	comprotocontracts.OnePhaseCommitProtocol(CPProxy{
		BeginTxFn:    txkit.Begin,
		CommitTxFn:   txkit.Commit,
		RollbackTxFn: txkit.Rollback,
	}).Spec(s)

	s.Test("on commit, no rollback is executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var i = 24
		t.Must.NoError(txkit.OnRollback(tx1, func() { i = 24 }))
		i = 42
		t.Must.NoError(txkit.Commit(tx1))
		t.Must.Equal(42, i)
	})

	s.Test("on rollback, rollback steps are executed in LIFO order", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		ns := make([]int, 0, 2)
		t.Must.NoError(txkit.OnRollback(tx1, func() { ns = append(ns, 42) }))
		t.Must.NoError(txkit.OnRollback(tx1, func() { ns = append(ns, 24) }))
		t.Must.NoError(txkit.Rollback(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

	s.Test("after rollback, adding further rollback steps yields a tx done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		t.Must.NoError(txkit.Rollback(ctx))
		t.Must.ErrorIs(txkit.ErrTxDone, txkit.OnRollback(ctx, func() {}))
	})

	s.Test("on multiple rollback, rollback steps are executed only once", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var n int
		t.Must.NoError(txkit.OnRollback(tx1, func() { n += 42 }))

		t.Must.NoError(txkit.Rollback(tx1))
		t.Must.Equal(42, n)

		t.Must.ErrorIs(txkit.ErrTxDone, txkit.Rollback(tx1))
		t.Must.Equal(42, n)
	})

	s.Test("on multiple commit, the second call yields tx is already done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		t.Must.NoError(txkit.Commit(ctx))
		t.Must.ErrorIs(txkit.ErrTxDone, txkit.Commit(ctx))
	})

	s.Test("on multiple rollback, the second call yields tx is already done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		t.Must.NoError(txkit.Rollback(ctx))
		t.Must.ErrorIs(txkit.ErrTxDone, txkit.Rollback(ctx))
	})

	s.Test("on multi tx stage, the most outer tx rollback override the commits", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var n int
		tx2, _ := txkit.Begin(tx1)
		t.Must.NoError(txkit.OnRollback(tx2, func() { n += 42 }))

		// commit in the inner layer
		t.Must.NoError(txkit.Commit(tx2))

		// rollback at higher level
		t.Must.NoError(txkit.Rollback(tx1))
		t.Must.Equal(42, n)
	})

	s.Test("on rollback, if rollback step encounters an error, it is propagated back as Rollback results", func(t *testcase.T) {
		ctx, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		expectedErr := t.Random.Error()
		t.Must.NoError(txkit.OnRollback(ctx, func(ctx context.Context) error { return expectedErr }))
		t.Must.ErrorIs(expectedErr, txkit.Rollback(ctx))
	})

	s.Test("on rollback, if parent rollback step encounters an error, it is propagated back as Rollback results", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		expectedErr := t.Random.Error()
		t.Must.NoError(txkit.OnRollback(tx1, func(context.Context) error { return expectedErr }))
		t.Must.ErrorIs(expectedErr, txkit.Rollback(tx2))
	})

	s.Test("on rollback, if rollback step encounters an error, it is propagated back through the Finish err pointer argument", func(t *testcase.T) {
		expectedErr := t.Random.Error()
		ctx, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		t.Must.NoError(txkit.OnRollback(ctx, func(context.Context) error { return expectedErr }))

		expectedOthErr := t.Random.Error()
		rErr := expectedOthErr
		txkit.Finish(&rErr, ctx)
		t.Must.Contain(rErr.Error(), expectedErr.Error())
		t.Must.Contain(rErr.Error(), expectedOthErr.Error())
	})

	s.Test("transaction allows concurrent interactions", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		defer txkit.Commit(tx1)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		makingSubTx := func() {
			var rErr error
			tx, berr := txkit.Begin(tx1)
			t.Should.NoError(berr)
			defer txkit.Finish(&rErr, tx)
		}
		addingRollbackStep := func() {
			t.Should.NoError(txkit.OnRollback(tx2, func() {}))
		}
		testcase.Race(makingSubTx, makingSubTx, addingRollbackStep, addingRollbackStep)
	})

	s.Test("during rollback, the original context is not yet cancelled", func(t *testcase.T) {
		ctx, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		t.Must.NoError(txkit.OnRollback(ctx, func(context.Context) error { return ctx.Err() }))
		t.Must.NoError(txkit.Rollback(ctx))
	})

	s.Test("committing the sub context should not cancel the parent context", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		t.Must.NoError(txkit.Commit(tx2))
		t.Must.Nil(tx1.Err())
		t.Must.NoError(txkit.Commit(tx1))
	})

	s.Test("suppose in a multi transaction setup, the context provided for a rollback step is not cancelled, even if committed context is", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		t.Must.NoError(txkit.OnRollback(tx2, func(ctx context.Context) error { return ctx.Err() }))
		t.Must.NoError(txkit.Commit(tx2))
		t.Must.Nil(tx1.Err())
		t.Must.NoError(txkit.Commit(tx1))
	})

	s.Test("suppose in a multi transaction setup, and the top level transaction is rolled back, the context provided for a rollback step is not cancelled", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		t.Must.NoError(txkit.OnRollback(tx2, func(ctx context.Context) error { return ctx.Err() }))
		t.Must.NoError(txkit.Commit(tx2), "commit successful TX2")
		t.Must.NoError(txkit.Rollback(tx1), "error at top level, rollback TX1")
	})

	s.Test("suppose a rollback is done in a sub tx, the top tx's Finish don't misinform a rollback error", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		t.Must.NoError(txkit.Rollback(tx2))
		var expectedErr error = errorkit.Error(t.Random.Error().Error())
		actualErr := expectedErr
		txkit.Finish(&actualErr, tx1)
		t.Must.Equal(expectedErr, actualErr)
	})

	s.Test("rollbacking back on multiple tx level yield no rollback error on Finish", func(t *testcase.T) {
		assertNoFinishErrOnRollback := func(ctx context.Context) {
			var expectedErr error = errorkit.Error(t.Random.Error().Error())
			actualErr := expectedErr
			txkit.Finish(&actualErr, ctx)
			t.Must.Equal(expectedErr, actualErr,
				"equality check intentionally to see no error wrapping is going on")
		}
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		tx2, err := txkit.Begin(tx1)
		t.Must.NoError(err)
		assertNoFinishErrOnRollback(tx2)
		assertNoFinishErrOnRollback(tx1)
	})

	s.Test("on rollback error, original value can be unwrapped", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		t.Must.NoError(err)
		expectedErr := errorkit.Error(t.Random.Error().Error())
		t.Must.NoError(txkit.OnRollback(tx1, func(ctx context.Context) error { return expectedErr }))
		var actualErr error = expectedErr
		txkit.Finish(&actualErr, tx1)
		t.Must.ErrorIs(expectedErr, actualErr)
	})

	s.Test("on context cancellation, the context of the rollback is still not cancelled", func(t *testcase.T) {
		ctx, cancel := context.WithCancel(context.Background())

		tx1, err := txkit.Begin(ctx)
		t.Must.NoError(err)
		t.Must.NoError(txkit.OnRollback(tx1, func(ctx context.Context) error { return ctx.Err() }))

		cancel()                            // cancel the context
		t.Must.NoError(txkit.Rollback(tx1)) // rollback the tx
	})
}

func MyActionWhichMutateTheSystemState(ctx context.Context) error {
	return nil
}

func RollbackForMyActionWhichMutatedTheSystemState(ctx context.Context) error {
	return nil
}

func MyUseCase(ctx context.Context) (returnErr error) {
	ctx, err := txkit.Begin(ctx)
	if err != nil {
		return err
	}
	defer txkit.Finish(&returnErr, ctx)

	if err := MyActionWhichMutateTheSystemState(ctx); err != nil {
		return err
	}

	txkit.OnRollback(ctx, func(ctx context.Context) error {
		return RollbackForMyActionWhichMutatedTheSystemState(ctx)
	})

	return nil
}

func Example_pkgLevelTxFunctions() {
	ctx := context.Background()
	ctx, err := txkit.Begin(ctx)
	if err != nil {
		logger.Error(ctx, "error with my tx", logging.ErrField(err))
	}

	if err := MyUseCase(ctx); err != nil {
		txkit.Rollback(ctx)
		return
	}
	txkit.Commit(ctx)
}

func Test_smoke(tt *testing.T) {
	t := testcase.NewT(tt, nil)

	ctx := context.Background()
	ns := make([]int, 0, 2)

	_ = func(ctx context.Context) (rerr error) {
		tx1, _ := txkit.Begin(ctx)
		defer txkit.Finish(&rerr, tx1)

		_ = func(ctx context.Context) (rerr error) {
			tx2, _ := txkit.Begin(ctx)
			defer txkit.Finish(&rerr, tx2)
			txkit.OnRollback(tx2, func() { ns = append(ns, 42) })
			txkit.OnRollback(tx2, func() { ns = append(ns, 24) })

			// err == nil ->> CommitTx because TxFinish
			return nil
		}(tx1)

		// err != nil ->> RollbackTx because TxFinish
		return fmt.Errorf("rollback this and anything that depends on our current tx")
	}(ctx)

	t.Must.Equal([]int{24, 42}, ns)
}

type CPProxy struct {
	BeginTxFn    func(ctx context.Context) (context.Context, error)
	CommitTxFn   func(ctx context.Context) error
	RollbackTxFn func(ctx context.Context) error
}

func (proxy CPProxy) BeginTx(ctx context.Context) (context.Context, error) {
	return proxy.BeginTxFn(ctx)
}

func (proxy CPProxy) CommitTx(ctx context.Context) error {
	return proxy.CommitTxFn(ctx)
}

func (proxy CPProxy) RollbackTx(ctx context.Context) error {
	return proxy.RollbackTxFn(ctx)
}
