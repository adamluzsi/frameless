package doubles_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/doubles"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase/assert"
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
		assert.Must(t).Equal(beginErr, rerr)
		assert.Must(t).Equal(commitErr, sopcp.CommitTx(ctx))
		assert.Must(t).Equal(rollbackErr, sopcp.RollbackTx(ctx))
		sopcp.BeginTxFunc = nil
		sopcp.CommitTxFunc = nil
		sopcp.RollbackTxFunc = nil
	})

	t.Run(`commit with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		assert.Must(t).Nil(err)
		m.Set(tx, `ns`, `key`, `value`)
		t.Cleanup(func() { m.Del(ctx, `ns`, `key`) })
		_, ok := m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
		_, ok = m.Get(tx, `ns`, `key`)
		assert.Must(t).True(ok)
		assert.Must(t).Nil(sopcp.CommitTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		assert.Must(t).True(ok)
	})

	t.Run(`rollback with embedded`, func(t *testing.T) {
		tx, err := sopcp.BeginTx(ctx)
		assert.Must(t).Nil(err)
		m.Set(tx, `ns`, `key`, `value`)
		_, ok := m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
		_, ok = m.Get(tx, `ns`, `key`)
		assert.Must(t).True(ok)
		assert.Must(t).Nil(sopcp.RollbackTx(tx))
		_, ok = m.Get(ctx, `ns`, `key`)
		assert.Must(t).False(ok)
	})
}
