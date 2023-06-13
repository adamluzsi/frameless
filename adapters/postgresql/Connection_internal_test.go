package postgresql

import (
	"context"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestConnection_Connection_noPanic(t *testing.T) {
	cm := connectionManager{}
	_, err := cm.connection(context.Background())
	assert.Error(t, err)
}

var _ Connection = (*connectionManager)(nil)
