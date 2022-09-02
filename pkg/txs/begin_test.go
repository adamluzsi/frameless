package txs_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/txs"
	comprotocontracts "github.com/adamluzsi/frameless/ports/comproto/contracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

func Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Context("cascading transaction", SpecCascading)
	s.Context("isolated transaction", SpecIsolated)
}

func SpecCascading(s *testcase.Spec) {
	comprotocontracts.OnePhaseCommitProtocol{
		Subject: func(tb testing.TB) comprotocontracts.OnePhaseCommitProtocolSubject {
			return CPProxy{
				BeginTxFn: func(ctx context.Context) (context.Context, error) {
					return txs.Begin(ctx), nil
				},
				CommitTxFn:   txs.CommitTx,
				RollbackTxFn: txs.RollbackTx,
			}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)

	s.Test("on commit, no rollback is executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.Begin(ctx)

		var i = 24
		t.Must.NoError(txs.OnRollback(tx1, func() { i = 24 }))
		i = 42
		t.Must.NoError(txs.CommitTx(tx1))
		t.Must.Equal(42, i)
	})

	s.Test("on rollback, rollback steps are executed in LIFO order", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.Begin(ctx)

		ns := make([]int, 0, 2)
		t.Must.NoError(txs.OnRollback(tx1, func() { ns = append(ns, 42) }))
		t.Must.NoError(txs.OnRollback(tx1, func() { ns = append(ns, 24) }))
		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

	s.Test("after rollback, adding further rollback steps yields a tx done error", func(t *testcase.T) {
		ctx := txs.Begin(context.Background())
		t.Must.NoError(txs.RollbackTx(ctx))
		t.Must.ErrorIs(txs.ErrTxDone, txs.OnRollback(ctx, func() {}))
	})

	s.Test("on multiple rollback, rollback steps are executed only once", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.Begin(ctx)

		var n int
		t.Must.NoError(txs.OnRollback(tx1, func() { n += 42 }))

		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal(42, n)

		t.Must.ErrorIs(txs.ErrTxDone, txs.RollbackTx(tx1))
		t.Must.Equal(42, n)
	})

	s.Test("on multi tx stage, the most outer tx rollback override the commits", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.Begin(ctx)

		var n int
		tx2 := txs.Begin(tx1)
		t.Must.NoError(txs.OnRollback(tx2, func() { n += 42 }))

		// commit in the inner layer
		t.Must.NoError(txs.CommitTx(tx2))

		// rollback at higher level
		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal(42, n)
	})
}

func SpecIsolated(s *testcase.Spec) {
	comprotocontracts.OnePhaseCommitProtocol{
		Subject: func(tb testing.TB) comprotocontracts.OnePhaseCommitProtocolSubject {
			return CPProxy{
				BeginTxFn: func(ctx context.Context) (context.Context, error) {
					return txs.BeginIsolated(ctx), nil
				},
				CommitTxFn:   txs.CommitTx,
				RollbackTxFn: txs.RollbackTx,
			}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)

	s.Test("on commit, no rollback is executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.BeginIsolated(ctx)

		var i = 24
		t.Must.NoError(txs.OnRollback(tx1, func() { i = 24 }))
		i = 42
		t.Must.NoError(txs.CommitTx(tx1))
		t.Must.Equal(42, i)
	})

	s.Test("on rollback, rollback steps are executed in LIFO order", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.BeginIsolated(ctx)

		ns := make([]int, 0, 2)
		t.Must.NoError(txs.OnRollback(tx1, func() { ns = append(ns, 42) }))
		t.Must.NoError(txs.OnRollback(tx1, func() { ns = append(ns, 24) }))
		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

	s.Test("after rollback, adding further rollback steps yields a tx done error", func(t *testcase.T) {
		ctx := txs.BeginIsolated(context.Background())
		t.Must.NoError(txs.RollbackTx(ctx))
		t.Must.ErrorIs(txs.ErrTxDone, txs.OnRollback(ctx, func() {}))
	})

	s.Test("on multiple rollback, rollback steps are executed only once", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.BeginIsolated(ctx)

		var n int
		t.Must.NoError(txs.OnRollback(tx1, func() { n += 42 }))

		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal(42, n)

		t.Must.ErrorIs(txs.ErrTxDone, txs.RollbackTx(tx1))
		t.Must.Equal(42, n)
	})

	s.Test("on multi tx stage, the most outer tx can't trigger rollback of the inner committed tx", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := txs.BeginIsolated(ctx)

		var n int
		tx2 := txs.BeginIsolated(tx1)
		t.Must.NoError(txs.OnRollback(tx2, func() { n += 42 }))

		// commit in the inner layers
		t.Must.NoError(txs.CommitTx(tx2))

		// rollback at higher level
		t.Must.NoError(txs.RollbackTx(tx1))
		t.Must.Equal(0, n)
	})
}

func Example_pkgLevelTxFunctions() {
	ctx := context.Background()

	_ = func(ctx context.Context) (rerr error) {
		tx := txs.Begin(ctx)
		defer txs.Finish(&rerr, tx)

		txs.OnRollback(tx, func() {
			// something to do
		})
		return nil
	}(ctx)
}

func Test_smoke(tt *testing.T) {
	t := testcase.NewT(tt, nil)

	ctx := context.Background()
	ns := make([]int, 0, 2)

	_ = func(ctx context.Context) (rerr error) {
		tx1 := txs.Begin(ctx)
		defer txs.Finish(&rerr, tx1)

		_ = func(ctx context.Context) (rerr error) {
			tx2 := txs.Begin(ctx)
			defer txs.Finish(&rerr, tx2)
			txs.OnRollback(tx2, func() { ns = append(ns, 42) })
			txs.OnRollback(tx2, func() { ns = append(ns, 24) })

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
