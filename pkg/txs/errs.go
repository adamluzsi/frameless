package txs

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errutils"
)

const (
	ErrTxDone errutils.Error = "transaction is already finished"
	ErrNoCtx  errutils.Error = "context.Context not given"
	ErrNoTx   errutils.Error = "no transaction present in the current context"
)

type txRollbackError struct {
	Err   error
	Cause error
}

func (err *txRollbackError) Error() string {
	return fmt.Sprintf("%s (rollback: %s)", err.Cause, err.Err)
}

func (err *txRollbackError) Unwrap() error {
	return err.Cause
}
