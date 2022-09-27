package postgresql_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	psh "github.com/adamluzsi/frameless/adapters/postgresql/spechelper"
	"github.com/adamluzsi/testcase/random"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var (
	_ postgresql.Connection = &sql.DB{}
	_ postgresql.Connection = &sql.Tx{}
)

func _() {
	var cm postgresql.ConnectionManager
	var _ interface {
		io.Closer
		Connection(ctx context.Context) (postgresql.Connection, error)
		comproto.OnePhaseCommitProtocol
	} = cm
}

func TestConnectionManager_Connection(t *testing.T) {
	ctx := context.Background()
	p := postgresql.NewConnectionManager(psh.DatabaseURL(t))

	connectionWithoutTx, err := p.Connection(ctx)
	require.NoError(t, err)
	require.Nil(t, connectionWithoutTx.QueryRowContext(ctx, "SELECT").Scan())

	connectionWithoutTxAgain, err := p.Connection(ctx)
	require.NoError(t, err)
	require.Nil(t, connectionWithoutTxAgain.QueryRowContext(ctx, "SELECT").Scan())

	ctxWithTx, err := p.BeginTx(ctx)
	require.Nil(t, err)
	defer func() { _ = p.RollbackTx(ctxWithTx) }()
	connectionWithTx, err := p.Connection(ctxWithTx)
	require.NoError(t, err)
	connectionWithTxAgain, err := p.Connection(ctxWithTx)
	require.NoError(t, err)
	require.Equal(t, connectionWithTx, connectionWithTxAgain)

	require.NotEqual(t, connectionWithTx, connectionWithoutTx)
}

func TestNewConnectionManager(t *testing.T) {
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	background := context.Background()
	c, err := cm.Connection(background)
	require.NoError(t, err)
	_, err = c.ExecContext(background, `SELECT TRUE`)
	require.NoError(t, err)
	require.NoError(t, cm.Close())
}

func TestConnectionManager_Close(t *testing.T) {
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	background := context.Background()
	c, err := cm.Connection(background)
	require.NoError(t, err)
	_, err = c.ExecContext(background, `SELECT TRUE`)
	require.NoError(t, err)
	require.NoError(t, cm.Close())
}

func TestConnectionManager_PoolContract(t *testing.T) {
	testcase.RunSuite(t, ConnectionManagerContract{
		Subject: func(tb testing.TB) postgresql.ConnectionManager {
			s := NewTestEntityRepository(t)
			return s.ConnectionManager
		},
		DriverName: "postgres",
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
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
	testcase.RunSuite(t, crudcontracts.OnePhaseCommitProtocol[psh.TestEntity, string]{
		Subject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[psh.TestEntity, string] {
			s := NewTestEntityRepository(tb)

			return crudcontracts.OnePhaseCommitProtocolSubject[psh.TestEntity, string]{
				Resource:      s,
				CommitManager: s,
			}
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeEnt: psh.MakeTestEntity,
	})
}

func TestConnectionManager_GetConnection_threadSafe(t *testing.T) {
	p := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	ctx := context.Background()
	blk := func() {
		_, err := p.Connection(ctx)
		require.Nil(t, err)
	}
	testcase.Race(blk, blk)
}

var _ testcase.Suite = ConnectionManagerContract{}

type ConnectionManagerContract struct {
	Subject func(tb testing.TB) postgresql.ConnectionManager
	MakeCtx func(testing.TB) context.Context

	DriverName string
	// CreateTable to create a dummy table with a specific name.
	// This is used to confirm transaction behaviors.
	CreateTable func(ctx context.Context, client postgresql.Connection, name string) error
	// DeleteTable to delete a previously made dummy table.
	DeleteTable func(ctx context.Context, client postgresql.Connection, name string) error
	// HasTable reports if a table exist with a given name.
	HasTable func(ctx context.Context, client postgresql.Connection, name string) (bool, error)
}

func (c ConnectionManagerContract) cm() testcase.Var[postgresql.ConnectionManager] {
	return testcase.Var[postgresql.ConnectionManager]{
		ID: "postgresql.ConnectionManager",
		Init: func(t *testcase.T) postgresql.ConnectionManager {
			return c.Subject(t)
		},
	}
}

func (c ConnectionManagerContract) Spec(s *testcase.Spec) {
	//s.Describe(`.DSN`, func(s *testcase.Spec) {
	//	subject := func(t *testcase.T) string {
	//		return c.cm().Get(t).DSN
	//	}
	//
	//	s.Then(`it should return data source name that is usable with sql.Open`, func(t *testcase.T) {
	//		db, err := sql.Open(c.DriverName, subject(t))
	//		require.NoError(t, err)
	//		t.Defer(db.Close)
	//		require.NotNil(t, db)
	//		require.Nil(t, db.Ping())
	//	})
	//})

	s.Describe(`.GetClient`, func(s *testcase.Spec) {
		ctx := s.Let("ctx", func(t *testcase.T) interface{} {
			return c.MakeCtx(t)
		})
		subject := func(t *testcase.T) (postgresql.Connection, error) {
			return c.cm().Get(t).Connection(ctx.Get(t).(context.Context))
		}

		s.Then(`it returns a client without an error`, func(t *testcase.T) {
			client, err := subject(t)
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	})

	s.Test(`.BeginTx + .GetClient = transaction`, func(t *testcase.T) {
		p := c.cm().Get(t)

		tx, err := p.BeginTx(c.MakeCtx(t))
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.Connection(tx)
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		ctx := c.MakeCtx(t)
		connection, err = p.Connection(ctx)
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it wasn't expected that the created dummy table present after rollback`)
	})

	s.Test(`.GetClient is in no transaction without context from a .BeginTx`, func(t *testcase.T) {
		p := c.cm().Get(t)

		ctx := c.MakeCtx(t)

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.Connection(ctx) // ctx -> no transaction
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		connection, err = p.Connection(ctx)
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present`)

		c.cleanupTable(t, name)
	})

	s.Test(`.BeginTx + .GetClient + .CommitTx`, func(t *testcase.T) {
		p := c.cm().Get(t)

		ctx := c.MakeCtx(t)

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		connection, err := p.Connection(tx)
		require.NoError(t, err)

		name := c.makeTestTableName()
		require.Nil(t, c.CreateTable(tx, connection, name))
		defer c.cleanupTable(t, name)

		connection, err = p.Connection(ctx) // in no comproto
		require.NoError(t, err)

		has, err := c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.False(t, has, `it was expected that the created dummy table is not observable outside of the transaction`)

		require.NoError(t, p.CommitTx(tx))

		connection, err = p.Connection(ctx)
		require.NoError(t, err)

		has, err = c.HasTable(ctx, connection, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present after commit`)

		c.cleanupTable(t, name)
	})
}

func (c ConnectionManagerContract) makeTestTableName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	return `test_` + random.New(random.CryptoSeed{}).StringNWithCharset(6, charset)
}

func (c ConnectionManagerContract) cleanupTable(t *testcase.T, name string) {
	ctx := c.MakeCtx(t)
	client, err := c.cm().Get(t).Connection(ctx)
	require.NoError(t, err)

	has, err := c.HasTable(ctx, client, name)
	require.NoError(t, err)
	if !has {
		return
	}

	require.Nil(t, c.DeleteTable(ctx, client, name))
}
