package iterate

import (
	"database/sql"

	"github.com/adamluzsi/frameless"
)

// type Rows
//     func (rs *Rows) Close() error
//     func (rs *Rows) ColumnTypes() ([]*ColumnType, error)
//     func (rs *Rows) Columns() ([]string, error)
//     func (rs *Rows) Err() error
//     func (rs *Rows) Next() bool
//     func (rs *Rows) NextResultSet() bool
// 	func (rs *Rows) Scan(dest ...interface{}) error

type SQLRowsDecoder func(destination interface{}, scan func(...interface{}) error) error

func OverSQLRows(rows *sql.Rows, decoder SQLRowsDecoder) frameless.Iterator {
	return &sqlRowsIterator{
		rows: rows,
	}
}

type sqlRowsIterator struct {
	rows    *sql.Rows
	decoder SQLRowsDecoder
}

func (this *sqlRowsIterator) Close() error {
	panic("not implemented")
	return this.rows.Close()
}

func (this *sqlRowsIterator) More() bool {
	panic("not implemented")
	return this.rows.Next()
}

func (this *sqlRowsIterator) Err() error {
	panic("not implemented")
	return this.rows.Err()
}

func (this *sqlRowsIterator) Decode(dest interface{}) error {
	panic("not implemented")
	return this.decoder(dest, this.rows.Scan)
}
