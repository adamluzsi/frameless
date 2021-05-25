package postgresql_test

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/postgresql"
	"io"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var _ interface {
	io.Closer
	// GetClient returns the current context's sql client.
	// This can be a simple *sql.DB or if we within a transaction, then an *sql.Tx
	GetConnection(ctx context.Context) (client postgresql.Connection, err error)
	frameless.OnePhaseCommitProtocol
} = &postgresql.ConnectionManager{}

var (
	_ postgresql.Connection = &sql.DB{}
	_ postgresql.Connection = &sql.Tx{}
)

func TestNewConnectionManager(t *testing.T) {
	cm := postgresql.NewConnectionManager(GetDatabaseURL(t))
	background := context.Background()
	c, err := cm.GetConnection(background)
	require.NoError(t, err)
	_, err = c.ExecContext(background, `SELECT TRUE`)
	require.NoError(t, err)
	require.NoError(t, cm.Close())
}

func TestConnectionManager_LookupTx(t *testing.T) {
	p := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}

	ctx := context.Background()
	ctxWithTx, err := p.BeginTx(ctx)
	require.Nil(t, err)
	defer func() { _ = p.RollbackTx(ctxWithTx) }()

	_, ok := p.LookupTx(context.Background())
	require.False(t, ok, `no tx expected in background`)

	client, err := p.GetConnection(ctxWithTx)
	require.NoError(t, err)

	txClient, ok := p.LookupTx(ctxWithTx)
	require.True(t, ok, `no tx expected in background`)

	require.Equal(t, client, txClient)
}

func TestConnectionManager_Close(t *testing.T) {
	cm := postgresql.NewConnectionManager(GetDatabaseURL(t))
	background := context.Background()
	c, err := cm.GetConnection(background)
	require.NoError(t, err)
	_, err = c.ExecContext(background, `SELECT TRUE`)
	require.NoError(t, err)
	require.NoError(t, cm.Close())
}

func TestConnectionManager_PoolContract(t *testing.T) {
	testcase.RunContract(t, PoolSpec{
		Subject: func(tb testing.TB) (*postgresql.ConnectionManager, contracts.CRD) {
			p := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}
			migrateEntityStorage(tb, p)
			s := &postgresql.Storage{
				T:                 StorageTestEntity{},
				ConnectionManager: p,
				Mapping:           StorageTestEntityMapping(),
			}
			return p, s
		},
		DriverName:     "postgres",
		FixtureFactory: fixtures.FixtureFactory{},
		CreateTable: func(ctx context.Context, client postgresql.Connection, name string) error {
			_, err := client.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %q ();`, name))
			return err
		},
		DeleteTable: func(ctx context.Context, client postgresql.Connection, name string) error {
			_, err := client.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %q;`, name))
			return err
		},
		HasTable: func(ctx context.Context, client postgresql.Connection, name string) (bool, error) {
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

func TestConnectionManager_OnePhaseCommitProtocolContract(t *testing.T) {
	testcase.RunContract(t, contracts.OnePhaseCommitProtocol{
		T: StorageTestEntity{},
		Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
			p := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}
			migrateEntityStorage(tb, p)

			s := &postgresql.Storage{
				T:                 StorageTestEntity{},
				ConnectionManager: p,
				Mapping:           StorageTestEntityMapping(),
			}
			return p, s
		},
		FixtureFactory: fixtures.FixtureFactory{},
	})
}

func TestConnectionManager_GetConnection_threadSafe(t *testing.T) {
	p := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}

	ctx := context.Background()
	testcase.Race(func() {
		_, err := p.GetConnection(ctx)
		require.Nil(t, err)
	})
}

type PoolSpec struct {
	Subject    func(tb testing.TB) (*postgresql.ConnectionManager, contracts.CRD)
	DriverName string
	contracts.FixtureFactory

	// CreateTable to create a dummy table with a specific name.
	// This is used to confirm transaction behaviors.
	CreateTable func(ctx context.Context, client postgresql.Connection, name string) error
	// DeleteTable to delete a previously made dummy table.
	DeleteTable func(ctx context.Context, client postgresql.Connection, name string) error
	// HasTable reports if a table exist with a given name.
	HasTable func(ctx context.Context, client postgresql.Connection, name string) (bool, error)
}

func (spec PoolSpec) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec PoolSpec) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec PoolSpec) cm() testcase.Var {
	return testcase.Var{
		Name: "*postgresql.ConnectionManager",
		Init: func(t *testcase.T) interface{} {
			pool, resource := spec.Subject(t)
			spec.resource().Set(t, resource)
			return pool
		},
	}
}

func (spec PoolSpec) cmGet(t *testcase.T) *postgresql.ConnectionManager {
	return spec.cm().Get(t).(*postgresql.ConnectionManager)
}

func (spec PoolSpec) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			_ = spec.cm().Get(t)
			return spec.resource().Get(t)
		},
	}
}

func (spec PoolSpec) resourceGet(t *testcase.T) contracts.CRD {
	return spec.resource().Get(t).(contracts.CRD)
}

func (spec PoolSpec) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)

	s.Describe(`.DSN`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) string {
			return spec.cmGet(t).DSN
		}

		s.Then(`it should return data source name that is usable with sql.Open`, func(t *testcase.T) {
			db, err := sql.Open(spec.DriverName, subject(t))
			require.NoError(t, err)
			t.Defer(db.Close)
			require.NotNil(t, db)
			require.Nil(t, db.Ping())
		})
	})

	s.Describe(`.GetClient`, func(s *testcase.Spec) {
		ctx := s.Let(`context`, func(t *testcase.T) interface{} {
			return spec.Context()
		})
		subject := func(t *testcase.T) (postgresql.Connection, error) {
			return spec.cmGet(t).GetConnection(ctx.Get(t).(context.Context))
		}

		s.Then(`it returns a client without an error`, func(t *testcase.T) {
			client, err := subject(t)
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	})

	s.Test(`.BeginTx + .GetClient = transaction`, func(t *testcase.T) {
		p := spec.cmGet(t)

		tx, err := p.BeginTx(spec.Context())
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(tx)
		require.NoError(t, err)

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, connection, name))
		defer spec.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		ctx := spec.Context()
		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err := spec.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it wasn't expected that the created dummy table present after rollback`)
	})

	s.Test(`.GetClient is in no transaction without context from a .BeginTx`, func(t *testcase.T) {
		p := spec.cmGet(t)

		ctx := spec.Context()

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(ctx) // ctx -> no transaction
		require.NoError(t, err)

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, connection, name))
		defer spec.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err := spec.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present`)

		spec.cleanupTable(t, name)
	})

	s.Test(`.BeginTx + .GetClient + .CommitTx`, func(t *testcase.T) {
		p := spec.cmGet(t)

		ctx := spec.Context()

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(tx)
		require.NoError(t, err)

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, connection, name))
		defer spec.cleanupTable(t, name)

		connection, err = p.GetConnection(ctx) // in no tx
		require.NoError(t, err)

		has, err := spec.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it was expected that the created dummy table is not observable outside of the transaction`)

		require.NoError(t, p.CommitTx(tx))

		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err = spec.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present after commit`)

		spec.cleanupTable(t, name)
	})
}

func (spec PoolSpec) makeTestTableName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	return `test_` + fixtures.Random.StringNWithCharset(6, charset)
}

func (spec PoolSpec) cleanupTable(t *testcase.T, name string) {
	ctx := spec.Context()
	client, err := spec.cmGet(t).GetConnection(ctx)
	require.NoError(t, err)

	has, err := spec.HasTable(ctx, client, name)
	require.NoError(t, err)
	if !has {
		return
	}

	require.Nil(t, spec.DeleteTable(ctx, client, name))
}
