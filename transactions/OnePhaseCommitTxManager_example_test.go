package transactions_test

import (
	"context"
	"database/sql"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/transactions"
)

func ExampleNewOnePhaseCommitTxManager() error {
	// during init
	var txm = transactions.NewOnePhaseCommitTxManager()
	var supplier = NewOPCExampleExternalResourceSupplier(txm)

	// during request without context level tx management
	var ctxWithoutTxManagement = context.Background()
	if err := supplier.DoSomething(ctxWithoutTxManagement); err != nil {
		return err
	}

	// during request with context level tx management
	var ctxWithTxManagement = context.Background()
	ctxWithTxManagement, handler := txm.ContextWithTransactionManagement(ctxWithTxManagement)
	if err := supplier.DoSomething(ctxWithoutTxManagement); err != nil {
		return handler.Rollback()
	}
	return handler.Commit()
}

func NewOPCExampleExternalResourceSupplier(txm *transactions.OnePhaseCommitTxManager) *OPCExampleExternalResourceSupplier {
	return &OPCExampleExternalResourceSupplier{ContextAccessor: txm.RegisterAdapter(OPCETXAdapter{})}
}

type OPCExampleExternalResourceSupplier struct {
	ContextAccessor transactions.ContextAccessor
}

func (s *OPCExampleExternalResourceSupplier) DoSomething(ctx context.Context) (rErr error) {
	tx, sf, err := s.ContextAccessor.FromContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		dErr := sf.Done()
		if rErr == nil {
			rErr = dErr // pass tx finalization err as return value
		}
	}()

	if _, err := tx.(*OPCETx).ExecContext(ctx, `SELECT 1`); err != nil {
		return err
	}
	return nil
}

type OPCETx struct {
	Err        error
	committed  bool
	rolledback bool
	id         string
}

func (tx *OPCETx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}

type OPCETXAdapter struct {
}

func (t OPCETXAdapter) BeginTx(context.Context) (ptr interface{}, err error) {
	return &OPCETx{id: fixtures.Random.String()}, nil
}

func (t OPCETXAdapter) Commit(ptr interface{}) error {
	ptr.(*OPCETx).committed = true
	return ptr.(*OPCETx).Err
}

func (t OPCETXAdapter) Rollback(ptr interface{}) error {
	ptr.(*OPCETx).rolledback = true
	return ptr.(*OPCETx).Err
}
