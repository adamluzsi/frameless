package postgresql

import (
	"context"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/testcase/assert"
	"io"
	"testing"
)

func TestConnectionManager_Connection_noPanic(t *testing.T) {
	cm := connectionManager{}
	_, err := cm.Connection(context.Background())
	assert.Error(t, err)
}

var _ interface {
	io.Closer
	Connection(ctx context.Context) (Connection, error)
	comproto.OnePhaseCommitProtocol
	Connection
} = (*connectionManager)(nil)
