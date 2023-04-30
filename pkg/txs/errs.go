package txs

import (
	"fmt"

	"github.com/adamluzsi/frameless/pkg/errorkit"
)

const (
	ErrTxDone errorkit.Error = "transaction is already finished"
	ErrNoCtx  errorkit.Error = "context.Context not given"
	ErrNoTx   errorkit.Error = "no transaction present in the current context"
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
