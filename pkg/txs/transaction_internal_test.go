package txs

var (
	_ transaction = (*baseTx)(nil)
	_ transaction = (*cascadingTx)(nil)
)
