package txs

import (
	"github.com/adamluzsi/frameless/pkg/errs"
	"github.com/adamluzsi/frameless/pkg/teardown"
)

type transaction struct {
	parent    *transaction
	done      bool
	cancel    func()
	rollbacks teardown.Teardown
}

func (tx *transaction) OnRollback(fn func() error) error {
	if tx.done {
		return ErrTxDone
	}
	tx.rollbacks.Defer(fn)
	return nil
}

func (tx *transaction) Commit() error {
	if err := tx.finish(); err != nil {
		return err
	}
	if tx.parent != nil {
		return tx.parent.OnRollback(tx.rollbacks.Finish)
	}
	return nil
}

func (tx *transaction) Rollback() (rErr error) {
	if err := tx.finish(); err != nil {
		return err
	}
	defer func() {
		if tx.parent == nil {
			return
		}
		pErr := tx.parent.Rollback()
		if pErr == nil {
			return
		}
		if rErr == nil {
			rErr = pErr
			return
		}
		rErr = errs.Errors{
			pErr,
			rErr,
		}
	}()
	return tx.rollbacks.Finish()
}

func (tx *transaction) isDone() bool {
	return tx.done
}

func (tx *transaction) finish() error {
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	tx.cancel()
	return nil
}
