package txs_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/txs"
	"github.com/adamluzsi/testcase"
	"testing"
)

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

func Test_pkgFn(tt *testing.T) {
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
