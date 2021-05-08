package postgresql_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources/postgresql"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func TestClientPool(t *testing.T) {
	s := testcase.NewSpec(t)

	dsn := s.Let(`data source name`, func(t *testcase.T) interface{} {
		return GetDatabaseURL(t)
	})
	clientPool := s.Let(`DefaultPool`, func(t *testcase.T) interface{} {
		return &postgresql.DefaultPool{
			DSN: dsn.Get(t).(string),
		}
	})
	clientPoolGet := func(t *testcase.T) *postgresql.DefaultPool {
		return clientPool.Get(t).(*postgresql.DefaultPool)
	}

	s.Describe(`.GetDSN`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) string {
			return clientPoolGet(t).GetDSN()
		}

		s.Test(`.GetDSN() is an attr accessor to .DSN`, func(t *testcase.T) {
			require.Equal(t, dsn.Get(t), subject(t))
		})
	})

}

func TestDefaultPool_LookupTx(t *testing.T) {
	p := &postgresql.DefaultPool{DSN: GetDatabaseURL(t)}

	ctx := context.Background()
	ctxWithTx, err := p.BeginTx(ctx)
	require.Nil(t, err)
	defer func() { _ = p.RollbackTx(ctxWithTx) }()

	_, ok := p.LookupTx(context.Background())
	require.False(t, ok, `no tx expected in background`)

	client, free, err := p.GetClient(ctxWithTx)
	require.NoError(t, err)
	defer free()

	txClient, ok := p.LookupTx(ctxWithTx)
	require.True(t, ok, `no tx expected in background`)

	require.Equal(t, client, txClient)
}
