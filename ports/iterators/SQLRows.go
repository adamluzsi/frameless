package iterators

import (
	"io"
)

func SQLRows[T any](rows sqlRows, mapper SQLRowMapper[T]) *SQLRowsIter[T] {
	return &SQLRowsIter[T]{Rows: rows, Mapper: mapper}
}

// SQLRowsIter allow you to use the same iterator pattern with sql.Rows structure.
// it allows you to do dynamic filtering, pipeline/middleware pattern on your sql results
// by using this wrapping around it.
// it also makes testing easier with the same Interface interface.
type SQLRowsIter[T any] struct {
	Rows   sqlRows
	Mapper SQLRowMapper[T]

	value T
	err   error
}

type sqlRows interface {
	io.Closer
	Next() bool
	Err() error
	Scan(dest ...interface{}) error
}

func (i *SQLRowsIter[T]) Close() error {
	return i.Rows.Close()
}

func (i *SQLRowsIter[T]) Next() bool {
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

func (i *SQLRowsIter[T]) Err() error {
	if i.err != nil {
		return i.err
	}
	return i.Rows.Err()
}

func (i *SQLRowsIter[T]) Value() T {
	return i.value
}

// sql rows iterator dependencies

type SQLRowScanner interface {
	Scan(...interface{}) error
}

type SQLRowMapper[T any] interface {
	Map(s SQLRowScanner) (T, error)
}

type SQLRowMapperFunc[T any] func(SQLRowScanner) (T, error)

func (fn SQLRowMapperFunc[T]) Map(s SQLRowScanner) (T, error) { return fn(s) }
