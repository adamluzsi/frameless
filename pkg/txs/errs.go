package txs

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/consterr"
)

const (
	ErrTxDone consterr.Error = "transaction is already finished"
	ErrNoTx   consterr.Error = "no transaction present in the current context"
)

type txRollbackError struct {
	Err   error
	Cause error
}

func (err *txRollbackError) Error() string {
	return fmt.Sprintf("%s (rollback: %s)", err.Cause, err.Err)
}
