package txs

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errs"
)

const (
	ErrTxDone errs.Error = "transaction is already finished"
	ErrNoCtx  errs.Error = "context.Context not given"
	ErrNoTx   errs.Error = "no transaction present in the current context"
)

type txRollbackError struct {
	Err   error
	Cause error
}

func (err *txRollbackError) Error() string {
	return fmt.Sprintf("%s (rollback: %s)", err.Cause, err.Err)
}
