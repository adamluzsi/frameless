package postgresql_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless"
	flcontracts "github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/postgresql"
	"github.com/adamluzsi/frameless/postgresql/contracts"
	"testing"

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

func TestDefaultPool_PoolContract(t *testing.T) {
	testcase.RunContract(t, contracts.Pool{
		Subject: func(tb testing.TB) (postgresql.Pool, flcontracts.CRD) {
			p := &postgresql.DefaultPool{DSN: GetDatabaseURL(t)}
			migrateEntityStorage(tb, p)
			s := &postgresql.Storage{
				T:       StorageTestEntity{},
				Pool:    p,
				Mapping: StorageTestEntityMapping(),
			}
			return p, s
		},
		DriverName:     "postgres",
		FixtureFactory: fixtures.FixtureFactory{},
		CreateTable: func(ctx context.Context, client postgresql.SQLClient, name string) error {
			_, err := client.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %q ();`, name))
			return err
		},
		DeleteTable: func(ctx context.Context, client postgresql.SQLClient, name string) error {
			_, err := client.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %q;`, name))
			return err
		},
		HasTable: func(ctx context.Context, client postgresql.SQLClient, name string) (bool, error) {
			var subquery string
			subquery += "SELECT FROM information_schema.tables"
			subquery += fmt.Sprintf("\nWHERE table_name = '%s'", name)
			query := fmt.Sprintf(`SELECT EXISTS (%s) AS e;`, subquery)

			var has bool
			err := client.QueryRowContext(ctx, query).Scan(&has)
			return has, err
		},
	})
}

func TestDefaultPool_OnePhaseCommitProtocolContract(t *testing.T) {
	testcase.RunContract(t, flcontracts.OnePhaseCommitProtocol{
		T: StorageTestEntity{},
		Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, flcontracts.CRD) {
			p := &postgresql.DefaultPool{DSN: GetDatabaseURL(t)}
			migrateEntityStorage(tb, p)

			s := &postgresql.Storage{
				T:       StorageTestEntity{},
				Pool:    p,
				Mapping: StorageTestEntityMapping(),
			}
			return p, s
		},
		FixtureFactory: fixtures.FixtureFactory{},
	})
}
