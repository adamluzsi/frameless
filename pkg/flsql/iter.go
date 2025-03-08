package flsql

import (
	"context"
	"database/sql"

	"go.llib.dev/frameless/pkg/iterkit"
)

func QueryMany[T any](c Queryable, ctx context.Context, mapper RowMapper[T], query string, args ...any) (iterkit.ErrIter[T], error) {
	rows, err := c.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return MakeRowsIterator(rows, mapper), nil
}

// MakeRowsIterator allow you to use the same iterator pattern with sql.Rows structure.
// it allows you to do dynamic filtering, pipeline/middleware pattern on your sql results
// by using this wrapping around it.
// it also makes testing easier with the same Interface interface.
func MakeRowsIterator[T any](rows Rows, mapper RowMapper[T]) iterkit.ErrIter[T] {
	if rows == nil {
		return iterkit.Empty2[T, error]()
	}
	return iterkit.Once2(func(yield func(T, error) bool) {
		defer rows.Close()
		for rows.Next() {
			if !yield(mapper.Map(rows)) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			var zero T
			if !yield(zero, err) {
				return
			}
		}
		if err := rows.Close(); err != nil {
			var zero T
			if !yield(zero, err) {
				return
			}
		}
	})
}

var _ Rows = (*sql.Rows)(nil)

type RowMapper[T any] func(Scanner) (T, error)

func (fn RowMapper[T]) Map(s Scanner) (T, error) {
	if fn == nil {
		panic("flsql RowMapper is missing")
	}
	return fn(s)
}
