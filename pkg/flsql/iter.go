package flsql

import (
	"database/sql"

	"go.llib.dev/frameless/pkg/iterkit"
)

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

type RowMapper[T any] interface {
	Map(s Scanner) (T, error)
}

type RowMapperFunc[T any] func(Scanner) (T, error)

func (fn RowMapperFunc[T]) Map(s Scanner) (T, error) { return fn(s) }
