package flsql

import (
	"database/sql"
	"io"

	"go.llib.dev/frameless/port/iterators"
)

func MakeSQLRowsIterator[T any](rows sqlRows, mapper SQLRowMapper[T]) iterators.Iterator[T] {
	return &sqlRowsIter[T]{Rows: rows, Mapper: mapper}
}

// sqlRowsIter allow you to use the same iterator pattern with sql.Rows structure.
// it allows you to do dynamic filtering, pipeline/middleware pattern on your sql results
// by using this wrapping around it.
// it also makes testing easier with the same Interface interface.
type sqlRowsIter[T any] struct {
	Rows   sqlRows
	Mapper SQLRowMapper[T]

	value T
	err   error
}

var _ sqlRows = (*sql.Rows)(nil)

type sqlRows interface {
	io.Closer
	Next() bool
	Err() error
	Scanner
}

func (i *sqlRowsIter[T]) Close() error {
	return i.Rows.Close()
}

func (i *sqlRowsIter[T]) Next() bool {
	if i.err != nil {
		return false
	}
	if !i.Rows.Next() {
		return false
	}
	v, err := i.Mapper.Map(i.Rows)
	if err != nil {
		i.err = err
		return false
	}
	i.value = v
	return true
}

func (i *sqlRowsIter[T]) Err() error {
	if i.err != nil {
		return i.err
	}
	return i.Rows.Err()
}

func (i *sqlRowsIter[T]) Value() T {
	return i.value
}

// sql rows iterator dependencies

type SQLRowMapper[T any] interface {
	Map(s Scanner) (T, error)
}

type SQLRowMapperFunc[T any] func(Scanner) (T, error)

func (fn SQLRowMapperFunc[T]) Map(s Scanner) (T, error) { return fn(s) }
