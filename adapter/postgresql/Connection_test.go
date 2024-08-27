package postgresql_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestConnect_smoke(t *testing.T) {
	ctx := context.Background()
	c, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)
	assert.NoError(t, c.QueryRowContext(ctx, "SELECT").Scan())
	assert.NoError(t, c.QueryRowContext(ctx, "SELECT").Scan())

	ctxWithTx, err := c.BeginTx(ctx)
	assert.NoError(t, err)
	defer func() { _ = c.RollbackTx(ctxWithTx) }()

	assert.NoError(t, c.QueryRowContext(ctxWithTx, "SELECT").Scan())
}

func TestConnect_smoke2(t *testing.T) {
	cm, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)
	background := context.Background()
	_, err = cm.ExecContext(background, `SELECT TRUE`)
	assert.NoError(t, err)
}

func TestConnection_PoolContract(t *testing.T) {
	testcase.RunSuite(t, ConnectionContract{
		MakeSubject: func(tb testing.TB) postgresql.Connection {
			cm, err := postgresql.Connect(DatabaseURL(tb))
			assert.NoError(tb, err)
			return cm
		},
		DriverName: "postgres",
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		CreateTable: func(ctx context.Context, connection postgresql.Connection, name string) error {
			_, err := connection.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %q ();`, name))
			return err
		},
		DeleteTable: func(ctx context.Context, connection postgresql.Connection, name string) error {
			_, err := connection.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %q;`, name))
			return err
		},
		HasTable: func(ctx context.Context, connection postgresql.Connection, name string) (bool, error) {
			var subquery string
			subquery += "SELECT FROM information_schema.tables"
			subquery += fmt.Sprintf("\nWHERE table_name = '%s'", name)
			query := fmt.Sprintf(`SELECT EXISTS (%s) AS e;`, subquery)

			var has bool
			err := connection.QueryRowContext(ctx, query).Scan(&has)
			return has, err
		},
	})
}

func TestConnection_OnePhaseCommitProtocolContract(t *testing.T) {
	repo := NewEntityRepository(t)
	crudcontracts.OnePhaseCommitProtocol[Entity, string](repo, repo.Connection).Test(t)
}

func Test_createSQLRowWithErr(t *testing.T) {
	err := errors.New("boom")
	var r sql.Row
	srrv := reflect.ValueOf(&r)
	reflectkit.SetValue(srrv.Elem().FieldByName("err"), reflect.ValueOf(err))
	v := srrv.Interface().(*sql.Row)
	assert.ErrorIs(t, err, v.Scan())
	assert.ErrorIs(t, err, v.Err())
}

func TestConnection_GetConnection_threadSafe(t *testing.T) {
	c, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)
	ctx := context.Background()
	blk := func() {
		_, err := c.ExecContext(ctx, "SELECT")
		assert.Nil(t, err)
	}
	testcase.Race(blk, blk)
}

var _ testcase.Suite = ConnectionContract{}

type ConnectionContract struct {
	MakeSubject func(tb testing.TB) postgresql.Connection
	MakeContext func(testing.TB) context.Context

	DriverName string
	// CreateTable to create a dummy table with a specific name.
	// This is used to confirm transaction behaviors.
	CreateTable func(ctx context.Context, client postgresql.Connection, name string) error
	// DeleteTable to delete a previously made dummy table.
	DeleteTable func(ctx context.Context, client postgresql.Connection, name string) error
	// HasTable reports if a table exist with a given name.
	HasTable func(ctx context.Context, client postgresql.Connection, name string) (bool, error)
}

func (c ConnectionContract) cm() testcase.Var[postgresql.Connection] {
	return testcase.Var[postgresql.Connection]{
		ID: "Connection",
		Init: func(t *testcase.T) postgresql.Connection {
			return c.MakeSubject(t)
		},
	}
}

func (c ConnectionContract) Spec(s *testcase.Spec) {
	s.Test(`.BeginTx = transaction`, func(t *testcase.T) {
		p := c.cm().Get(t)

		tx, err := p.BeginTx(c.MakeContext(t))
		assert.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		name := c.makeTestTableName()
		assert.Nil(t, c.CreateTable(tx, p, name))
		defer c.cleanupTable(t, name)

		assert.NoError(t, p.RollbackTx(tx))

		ctx := c.MakeContext(t)
		has, err := c.HasTable(ctx, p, name)
		assert.NoError(t, err)
		assert.False(t, has, `it wasn't expected that the created dummy table present after rollback`)
	})

	s.Test(`no transaction without context from a .BeginTx`, func(t *testcase.T) {
		p := c.cm().Get(t)

		ctx := c.MakeContext(t)

		tx, err := p.BeginTx(ctx)
		assert.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		name := c.makeTestTableName()
		assert.Nil(t, c.CreateTable(ctx, p, name))
		defer c.cleanupTable(t, name)

		assert.NoError(t, p.RollbackTx(tx))

		has, err := c.HasTable(ctx, p, name)
		assert.NoError(t, err)
		assert.True(t, has, `it was expected that the created dummy table present`)

		c.cleanupTable(t, name)
	})

	s.Test(`.BeginTx + .CommitTx`, func(t *testcase.T) {
		p := c.cm().Get(t)

		ctx := c.MakeContext(t)

		tx, err := p.BeginTx(ctx)
		assert.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		name := c.makeTestTableName()
		assert.Nil(t, c.CreateTable(tx, p, name))
		defer c.cleanupTable(t, name)

		has, err := c.HasTable(ctx, p, name)
		assert.NoError(t, err)
		assert.False(t, has, `it was expected that the created dummy table is not observable outside of the transaction`)

		assert.NoError(t, p.CommitTx(tx))

		has, err = c.HasTable(ctx, p, name)
		assert.NoError(t, err)
		assert.True(t, has, `it was expected that the created dummy table present after commit`)

		c.cleanupTable(t, name)
	})
}

func (c ConnectionContract) makeTestTableName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	return `test_` + random.New(random.CryptoSeed{}).StringNWithCharset(6, charset)
}

func (c ConnectionContract) cleanupTable(t *testcase.T, name string) {
	ctx := c.MakeContext(t)

	has, err := c.HasTable(ctx, c.cm().Get(t), name)
	assert.NoError(t, err)
	if !has {
		return
	}

	assert.Nil(t, c.DeleteTable(ctx, c.cm().Get(t), name))
}
