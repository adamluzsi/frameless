package doubles_test

import (
	"context"
	"fmt"
	"testing"

	"go.llib.dev/frameless/internal/doubles"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/testcase/assert"
)

func TestStubOnePhaseCommitProtocol_smoke(t *testing.T) {
	m := memory.NewMemory()

	var (
		beginErr    = fmt.Errorf("BeginTxFunc")
		commitErr   = fmt.Errorf("CommitTxFunc")
		rollbackErr = fmt.Errorf("RollbackTxFunc")
	)
	sopcp := &doubles.StubOnePhaseCommitProtocol{
		OnePhaseCommitProtocol: m,
		BeginTxFunc:            func(ctx context.Context) (context.Context, error) { return ctx, beginErr },
		CommitTxFunc:           func(ctx context.Context) error { return commitErr },
		RollbackTxFunc:         func(ctx context.Context) error { return rollbackErr },
	}

	ctx := context.Background()

	t.Run(`stub works when set`, func(t *testing.T) {
		_, rerr := sopcp.BeginTx(ctx)
		assert.Equal(t, beginErr, rerr)
		assert.Equal(t, commitErr, sopcp.CommitTx(ctx))
		assert.Equal(t, rollbackErr, sopcp.RollbackTx(ctx))
		sopcp.BeginTxFunc = nil
		sopcp.CommitTxFunc = nil
		sopcp.RollbackTxFunc = nil
	})

	t.Run(`commit with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		assert.Must(t).NoError(err)
		m.Set(tx, `ns`, `key`, `value`)
		t.Cleanup(func() { m.Del(ctx, `ns`, `key`) })
		_, ok := m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
		_, ok = m.Get(tx, `ns`, `key`)
		assert.True(t, ok)
		assert.Must(t).NoError(sopcp.CommitTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		assert.True(t, ok)
	})

	t.Run(`rollback with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		assert.Must(t).NoError(err)
		m.Set(tx, `ns`, `key`, `value`)
		_, ok := m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
		_, ok = m.Get(tx, `ns`, `key`)
		assert.True(t, ok)
		assert.Must(t).NoError(sopcp.RollbackTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
	})
}
