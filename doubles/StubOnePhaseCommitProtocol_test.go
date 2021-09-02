package doubles_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/stretchr/testify/require"
)

func TestStubOnePhaseCommitProtocol_smoke(t *testing.T) {
	m := inmemory.NewMemory()

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
		require.Equal(t, beginErr, rerr)
		require.Equal(t, commitErr, sopcp.CommitTx(ctx))
		require.Equal(t, rollbackErr, sopcp.RollbackTx(ctx))
		sopcp.BeginTxFunc = nil
		sopcp.CommitTxFunc = nil
		sopcp.RollbackTxFunc = nil
	})

	t.Run(`commit with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		require.NoError(t, err)
		m.Set(tx, `ns`, `key`, `value`)
		t.Cleanup(func() { m.Del(ctx, `ns`, `key`) })
		_, ok := m.Get(ctx, `ns`, `key`)
		require.False(t, ok)
		_, ok = m.Get(tx, `ns`, `key`)
		require.True(t, ok)
		require.NoError(t, sopcp.CommitTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		require.True(t, ok)
	})

	t.Run(`rollback with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		require.NoError(t, err)
		m.Set(tx, `ns`, `key`, `value`)
		_, ok := m.Get(ctx, `ns`, `key`)
		require.False(t, ok)
		_, ok = m.Get(tx, `ns`, `key`)
		require.True(t, ok)
		require.NoError(t, sopcp.RollbackTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		require.False(t, ok)
	})
}
