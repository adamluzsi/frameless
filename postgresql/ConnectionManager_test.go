package postgresql_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/postgresql"

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
	testcase.RunContract(t, ConnectionManagerSpec{
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
		DriverName: "postgres",
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
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
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	})
}

func TestConnectionManager_GetConnection_threadSafe(t *testing.T) {
	p := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}
	ctx := context.Background()
	blk := func() {
		_, err := p.GetConnection(ctx)
		require.Nil(t, err)
	}
	testcase.Race(blk, blk)
}

var _ testcase.Contract = ConnectionManagerSpec{}

type ConnectionManagerSpec struct {
	Subject        func(tb testing.TB) (*postgresql.ConnectionManager, contracts.CRD)
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
	DriverName     string

	// CreateTable to create a dummy table with a specific name.
	// This is used to confirm transaction behaviors.
	CreateTable func(ctx context.Context, client postgresql.Connection, name string) error
	// DeleteTable to delete a previously made dummy table.
	DeleteTable func(ctx context.Context, client postgresql.Connection, name string) error
	// HasTable reports if a table exist with a given name.
	HasTable func(ctx context.Context, client postgresql.Connection, name string) (bool, error)
}

func (c ConnectionManagerSpec) cm() testcase.Var {
	return testcase.Var{
		Name: "*postgresql.ConnectionManager",
		Init: func(t *testcase.T) interface{} {
			pool, resource := c.Subject(t)
			c.resource().Set(t, resource)
			return pool
		},
	}
}

func (c ConnectionManagerSpec) cmGet(t *testcase.T) *postgresql.ConnectionManager {
	return c.cm().Get(t).(*postgresql.ConnectionManager)
}

func (c ConnectionManagerSpec) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			_ = c.cm().Get(t)
			return c.resource().Get(t)
		},
	}
}

func (c ConnectionManagerSpec) resourceGet(t *testcase.T) contracts.CRD {
	return c.resource().Get(t).(contracts.CRD)
}

func (c ConnectionManagerSpec) factory() testcase.Var {
	return testcase.Var{
		Name: "factory",
		Init: func(t *testcase.T) interface{} {
			return c.FixtureFactory(t)
		},
	}
}
func (c ConnectionManagerSpec) factoryGet(t *testcase.T) contracts.FixtureFactory {
	return c.factory().Get(t).(contracts.FixtureFactory)
}

func (c ConnectionManagerSpec) Spec(s *testcase.Spec) {
	s.Describe(`.DSN`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) string {
			return c.cmGet(t).DSN
		}

		s.Then(`it should return data source name that is usable with sql.Open`, func(t *testcase.T) {
			db, err := sql.Open(c.DriverName, subject(t))
			require.NoError(t, err)
			t.Defer(db.Close)
			require.NotNil(t, db)
			require.Nil(t, db.Ping())
		})
	})

	s.Describe(`.GetClient`, func(s *testcase.Spec) {
		ctx := s.Let(`context`, func(t *testcase.T) interface{} {
			return c.Context(t)
		})
		subject := func(t *testcase.T) (postgresql.Connection, error) {
			return c.cmGet(t).GetConnection(ctx.Get(t).(context.Context))
		}

		s.Then(`it returns a client without an error`, func(t *testcase.T) {
			client, err := subject(t)
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	})

	s.Test(`.BeginTx + .GetClient = transaction`, func(t *testcase.T) {
		p := c.cmGet(t)

		tx, err := p.BeginTx(c.Context(t))
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(tx)
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		ctx := c.Context(t)
		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it wasn't expected that the created dummy table present after rollback`)
	})

	s.Test(`.GetClient is in no transaction without context from a .BeginTx`, func(t *testcase.T) {
		p := c.cmGet(t)

		ctx := c.Context(t)

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(ctx) // ctx -> no transaction
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present`)

		c.cleanupTable(t, name)
	})

	s.Test(`.BeginTx + .GetClient + .CommitTx`, func(t *testcase.T) {
		p := c.cmGet(t)

		ctx := c.Context(t)

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.GetConnection(tx)
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		connection, err = p.GetConnection(ctx) // in no tx
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it was expected that the created dummy table is not observable outside of the transaction`)

		require.NoError(t, p.CommitTx(tx))

		connection, err = p.GetConnection(ctx)
		require.NoError(t, err)

		has, err = c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present after commit`)

		c.cleanupTable(t, name)
	})
}

func (c ConnectionManagerSpec) makeTestTableName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	return `test_` + fixtures.Random.StringNWithCharset(6, charset)
}

func (c ConnectionManagerSpec) cleanupTable(t *testcase.T, name string) {
	ctx := c.Context(t)
	client, err := c.cmGet(t).GetConnection(ctx)
	require.NoError(t, err)

	has, err := c.HasTable(ctx, client, name)
	require.NoError(t, err)
	if !has {
		return
	}

	require.Nil(t, c.DeleteTable(ctx, client, name))
}
