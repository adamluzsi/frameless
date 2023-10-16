package comproto

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/errorkit"
)

func FinishTx(errp *error, commit, rollback func() error) {
	if errp == nil {
		panic(fmt.Errorf(`error pointer cannot be nil for Finish Tx methods`))
	}
	if *errp != nil {
		*errp = errorkit.Merge(*errp, rollback())
		return
	}
	*errp = commit()
}

func FinishOnePhaseCommit(errp *error, cm OnePhaseCommitProtocol, tx context.Context) {
	FinishTx(errp, func() error {
		return cm.CommitTx(tx)
	}, func() error {
		return cm.RollbackTx(tx)
	})
}
