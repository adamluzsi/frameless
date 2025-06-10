package postgresql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/flsql"
)

type Connection struct {
	flsql.ConnectionAdapter[pgxpool.Pool, pgx.Tx]
}

func Connect(dsn string) (Connection, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return Connection{}, err
	}
	return Connection{
		ConnectionAdapter: flsql.ConnectionAdapter[pgxpool.Pool, pgx.Tx]{
			DB: pool,

			DBAdapter: func(db *pgxpool.Pool) flsql.Queryable {
				return pgxQueryableAdapter[*pgxpool.Pool]{Q: db}
			},
			TxAdapter: func(tx *pgx.Tx) flsql.Queryable {
				return pgxQueryableAdapter[pgx.Tx]{Q: *tx}
			},

			Begin: func(ctx context.Context, db *pgxpool.Pool) (*pgx.Tx, error) {
				var (
					tx  pgx.Tx
					err error
				)
				if opts, ok := ContextTxOptions.Lookup(ctx); ok {
					tx, err = db.BeginTx(ctx, opts)
				} else {
					tx, err = db.Begin(ctx)
				}
				if err != nil {
					return nil, err
				}
				return &tx, nil
			},

			Commit: func(ctx context.Context, tx *pgx.Tx) error {
				return (*tx).Commit(ctx)
			},

			Rollback: func(ctx context.Context, tx *pgx.Tx) error {
				return (*tx).Rollback(ctx)
			},

			OnClose: func() error {
				pool.Close()
				return nil
			},
		},
	}, nil
}

var ContextTxOptions contextkit.ValueHandler[ctxKeyTxOptions, pgx.TxOptions]

type ctxKeyTxOptions struct{}

type pgxQueryableAdapter[Q pgxQueryable] struct{ Q Q }

type pgxQueryable interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (ca pgxQueryableAdapter[Q]) ExecContext(ctx context.Context, query string, args ...interface{}) (flsql.Result, error) {
	r, err := ca.Q.Exec(ctx, query, args...)
	return sqlResultAdapter{CommandTag: r}, err
}

type sqlResultAdapter struct{ pgconn.CommandTag }

func (a sqlResultAdapter) RowsAffected() (int64, error) {
	return a.CommandTag.RowsAffected(), nil
}

func (ca pgxQueryableAdapter[Q]) QueryContext(ctx context.Context, query string, args ...interface{}) (flsql.Rows, error) {
	rows, err := ca.Q.Query(ctx, query, args...)
	return pgxRowsAdapter{Rows: rows}, err
}

func (ca pgxQueryableAdapter[Q]) QueryRowContext(ctx context.Context, query string, args ...interface{}) flsql.Row {
	return ca.Q.QueryRow(ctx, query, args...)
}

type pgxRowsAdapter struct{ pgx.Rows }

func (a pgxRowsAdapter) Close() error {
	a.Rows.Close()
	return nil // TODO: return a.Rows.Err()
}
