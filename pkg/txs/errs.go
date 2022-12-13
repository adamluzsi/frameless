package txs

import (
	"fmt"

	"github.com/adamluzsi/frameless/pkg/errorutil"
)

const (
	ErrTxDone errorutil.Error = "transaction is already finished"
	ErrNoCtx  errorutil.Error = "context.Context not given"
	ErrNoTx   errorutil.Error = "no transaction present in the current context"
)

type TxRollbackError struct {
	Err   error
	Cause error
}

func (err *TxRollbackError) Error() string {
	return fmt.Sprintf("%s (rollback: %s)", err.Cause, err.Err)
}

func (err *TxRollbackError) Unwrap() error {
	return err.Cause
}
