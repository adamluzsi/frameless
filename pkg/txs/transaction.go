package txs

import (
	"github.com/adamluzsi/frameless/pkg/teardown"
)

type transaction interface {
	LookupParent() (transaction, bool)
	Commit() error
	Rollback() error
	OnRollback(func()) error
}

type baseTx struct {
	parent    transaction
	done      bool
	cancel    func()
	rollbacks teardown.Teardown
}

func (tx *baseTx) LookupParent() (transaction, bool) {
	return tx.parent, tx.parent != nil
}

func (tx *baseTx) Commit() error {
	return tx.finish()
}

func (tx *baseTx) Rollback() error {
	if err := tx.finish(); err != nil {
		return err
	}
	tx.rollbacks.Finish()
	return nil
}

func (tx *baseTx) OnRollback(fn func()) error {
	if tx.done {
		return ErrTxDone
	}
	tx.rollbacks.Defer(fn)
	return nil
}

func (tx *baseTx) finish() error {
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	tx.cancel()
	return nil
}

type cascadingTx struct {
	baseTx
}

func (tx *cascadingTx) Commit() error {
	if err := tx.finish(); err != nil {
		return err
	}
	if parent, ok := tx.LookupParent(); ok {
		return parent.OnRollback(tx.rollbacks.Finish)
	}
	return nil
}

func (tx *cascadingTx) Rollback() error {
	if err := tx.baseTx.Rollback(); err != nil {
		return err
	}
	if parent, ok := tx.LookupParent(); ok {
		return parent.Rollback()
	}
	return nil
}
