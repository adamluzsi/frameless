package iterators

import (
	"github.com/adamluzsi/frameless"
	"io"
)

func NewSQLRows(rows SQLRows, mapper SQLRowMapper) *SQLRowsIterator {
	return &SQLRowsIterator{rows: rows, mapper: mapper,}
}

// SQLRowsIterator allow you to use the same iterator pattern with sql.Rows structure.
// it allows you to do dynamic filtering, pipeline/middleware pattern on your sql results
// by using this wrapping around it.
// it also makes testing easier with the same frameless.Iterator interface.
type SQLRowsIterator struct {
	rows   SQLRows
	mapper SQLRowMapper
}

func (i *SQLRowsIterator) Close() error {
	return i.rows.Close()
}

func (i *SQLRowsIterator) Next() bool {
	return i.rows.Next()
}

func (i *SQLRowsIterator) Err() error {
	return i.rows.Err()
}

func (i *SQLRowsIterator) Decode(e frameless.Entity) error {
	return i.mapper.Map(i.rows, e)
}

// sql rows iterator dependencies

type SQLRowScanner interface {
	Scan(...interface{}) error
}

type SQLRowMapper interface {
	Map(s SQLRowScanner, e frameless.Entity) error
}

type SQLRowMapperFunc func(SQLRowScanner, frameless.Entity) error

func (fn SQLRowMapperFunc) Map(s SQLRowScanner, e frameless.Entity) error {
	return fn(s, e)
}

type SQLRows interface {
	io.Closer
	Next() bool
	Err() error
	Scan(dest ...interface{}) error
}
