package txs

import "github.com/adamluzsi/frameless/pkg/consterr"

const (
	ErrTxDone consterr.Error = "transaction is already finished"
	ErrNoTx   consterr.Error = "no transaction present in the current context"
)
