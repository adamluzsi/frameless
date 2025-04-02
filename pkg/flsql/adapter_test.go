package flsql_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/port/comproto/comprotocontracts"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var _ flsql.Connection = flsql.ConnectionAdapter[any, any]{}
var _ flsql.Queryable = flsql.QueryableAdapter{}

func ExampleConnectionAdapter() {
	db, err := sql.Open("dbname", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	_ = flsql.ConnectionAdapter[sql.DB, sql.Tx]{
		DB: db,

		DBAdapter: flsql.QueryableSQL[*sql.DB],
		TxAdapter: flsql.QueryableSQL[*sql.Tx],

		Begin: func(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
			// TODO: integrate begin tx options
			return db.BeginTx(ctx, nil)
		},

		Commit: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Commit()
		},

		Rollback: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Rollback()
		},
	}
}

func ExampleSQLConnectionAdapter() {
	db, err := sql.Open("dbname", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	_ = flsql.SQLConnectionAdapter(db)
}

func TestQueryableAdapter_ExecContext(t *testing.T) {
	// Arrange

	res := &mockResult{StubRowsAffected: int64(rnd.Int())}

	mockExecFunc := func(ctx context.Context, query string, args ...interface{}) (flsql.Result, error) {
		return res, nil // mock result
	}
	adapter := flsql.QueryableAdapter{
		ExecFunc: mockExecFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	result, err := adapter.ExecContext(ctx, query)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal[flsql.Result](t, res, result)
}

type mockResult struct{ StubRowsAffected int64 }

func (m *mockResult) RowsAffected() (int64, error) { return m.StubRowsAffected, nil }

func TestQueryableAdapter_QueryContext(t *testing.T) {
	// Arrange
	expRows := &mockRows{StubErr: rnd.Error()}
	expErr := rnd.Error()
	mockQueryFunc := func(ctx context.Context, query string, args ...interface{}) (flsql.Rows, error) {
		return expRows, expErr
	}
	adapter := flsql.QueryableAdapter{
		QueryFunc: mockQueryFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	rows, err := adapter.QueryContext(ctx, query)

	// Assert
	assert.ErrorIs(t, err, expErr)
	assert.Equal[flsql.Rows](t, expRows, rows)
}

type mockRows struct {
	StubErr  error
	StubNext func() bool
	StubScan func(dest ...any) error
}

func (m *mockRows) Err() error {
	return m.StubErr
}

func (m *mockRows) Next() bool {
	if m.StubNext != nil {
		return m.StubNext()
	}
	return false
}

func (m *mockRows) Scan(dest ...any) error {
	if m.StubScan != nil {
		return m.StubScan(dest...)
	}
	return nil
}

func (m *mockRows) Close() error { return nil }

func TestQueryableAdapter_QueryRowContext(t *testing.T) {
	// Arrange
	expErr := rnd.Error()
	expRow := &mockRow{StubScan: func(dest ...any) error { return expErr }}
	expArgs := []any{rnd.String(), rnd.Int()}

	mockQueryRowFunc := func(ctx context.Context, query string, args ...interface{}) flsql.Row {
		assert.Equal(t, expArgs, args)
		return expRow // mock row
	}
	adapter := flsql.QueryableAdapter{
		QueryRowFunc: mockQueryRowFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	row := adapter.QueryRowContext(ctx, query, expArgs...)

	// Assert
	assert.NotNil(t, row)
	assert.ErrorIs(t, row.Scan(), expErr)
}

type mockRow struct {
	StubScan func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.StubScan != nil {
		return m.StubScan(dest...)
	}
	return nil
}

func TestConnectionAdapter_LookupTx(t *testing.T) {
	type DB struct {
		N string
		V string
	}
	type TX struct {
		ID string
		V  string
	}
	type Queryable struct {
		V string
		F any
	}

	rnd := random.New(random.CryptoSeed{})
	db := &DB{
		N: rnd.Domain(),
		V: rnd.String(),
	}

	subject := flsql.ConnectionAdapter[DB, TX]{
		DB: db,
		DBAdapter: func(db *DB) flsql.Queryable {
			return flsql.QueryableAdapter{}
		},
		TxAdapter: func(tx *TX) flsql.Queryable {
			return flsql.QueryableAdapter{}
		},
		Begin: func(ctx context.Context, db *DB) (*TX, error) {
			return &TX{ID: rnd.UUID(), V: db.V}, nil
		},
		Commit: func(ctx context.Context, tx *TX) error {
			assert.Equal(t, tx.V, db.V)
			return nil
		},
		Rollback: func(ctx context.Context, tx *TX) error {
			assert.Equal(t, tx.V, db.V)
			return nil
		},
	}

	t.Run("comprotocontracts.OnePhaseCommitProtocol",
		comprotocontracts.OnePhaseCommitProtocol(subject).Test)

	t.Run("smoke", func(t *testing.T) {
		ctx := context.Background()

		tx, ok := subject.LookupTx(ctx)
		var _ *TX = tx
		assert.False(t, ok)
		assert.Nil(t, tx)

		ctx, err := subject.BeginTx(ctx)
		assert.NoError(t, err)

		tx, ok = subject.LookupTx(ctx)
		assert.True(t, ok)
		assert.NotNil(t, tx)
		assert.Equal(t, tx.V, db.V)
	})

	t.Run("cancel will not cannel the context of the Rollback call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		var subject flsql.ConnectionAdapter[DB, TX] = subject // pass by value copy
		ogRollback := subject.Rollback
		subject.Rollback = func(ctx context.Context, tx *TX) error {
			assert.NoError(t, ctx.Err())
			return ogRollback(ctx, tx)
		}

		ctx, err := subject.BeginTx(ctx)
		assert.NoError(t, err)

		cancel()
		assert.ErrorIs(t, ctx.Err(), subject.RollbackTx(ctx))
	})
}
