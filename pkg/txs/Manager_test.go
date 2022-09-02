package txs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/txs"
	comprotocontracts "github.com/adamluzsi/frameless/ports/comproto/contracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestManager(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *txs.Manager {
		return &txs.Manager{ContextKey: t.Random.String()}
	})

	s.Context("supplies basic one-phase commit protocol behaviour", comprotocontracts.OnePhaseCommitProtocol{
		Subject: func(tb testing.TB) comprotocontracts.OnePhaseCommitProtocolSubject {
			return txs.Manager{}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec)

	s.Test("on commit, no rollback is executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := subject.Get(t).Begin(ctx)

		var i = 24
		t.Must.NoError(subject.Get(t).OnRollback(tx1, func() { i = 24 }))
		i = 42
		t.Must.NoError(subject.Get(t).CommitTx(tx1))
		t.Must.Equal(42, i)
	})

	s.Test("on rollback, rollback steps are executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := subject.Get(t).Begin(ctx)

		ns := make([]int, 0, 2)
		t.Must.NoError(subject.Get(t).OnRollback(tx1, func() { ns = append(ns, 42) }))
		t.Must.NoError(subject.Get(t).OnRollback(tx1, func() { ns = append(ns, 24) }))
		t.Must.NoError(subject.Get(t).RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

	s.Test("on multiple rollback, rollback steps are executed only once", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := subject.Get(t).Begin(ctx)

		ns := make([]int, 0, 2)
		t.Must.NoError(subject.Get(t).OnRollback(tx1, func() { ns = append(ns, 42) }))
		t.Must.NoError(subject.Get(t).OnRollback(tx1, func() { ns = append(ns, 24) }))

		t.Must.NoError(subject.Get(t).RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)

		t.Must.NotNil(subject.Get(t).RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

	s.Test("on multi tx stage, the most outer tx rollback override the commits", func(t *testcase.T) {
		ctx := context.Background()
		tx1 := subject.Get(t).Begin(ctx)

		ns := make([]int, 0, 2)
		tx2 := subject.Get(t).Begin(tx1)
		t.Must.NoError(subject.Get(t).OnRollback(tx2, func() { ns = append(ns, 42) }))
		t.Must.NoError(subject.Get(t).OnRollback(tx2, func() { ns = append(ns, 24) }))

		// commit in the inner layer
		t.Must.NoError(subject.Get(t).CommitTx(tx2))
		t.Must.ErrorIs(txs.ErrTxDone, subject.Get(t).CommitTx(tx2))

		// rollback at higher level
		t.Must.NoError(subject.Get(t).RollbackTx(tx1))
		t.Must.Equal([]int{24, 42}, ns)
	})

}
