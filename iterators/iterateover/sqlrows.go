package iterateover

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators/iterateover/sqlrows"
)

func SQLRows(rows sqlrows.Rows, decoder sqlrows.Decoder) frameless.Iterator {
	return &sqlRowsIterator{rows: rows, decoder: decoder}
}

type sqlRowsIterator struct {
	rows    sqlrows.Rows
	decoder sqlrows.Decoder
}

func (this *sqlRowsIterator) Close() error {
	return this.rows.Close()
}

func (this *sqlRowsIterator) Err() error {
	return this.rows.Err()
}

func (this *sqlRowsIterator) Next() bool {
	return this.rows.Next()
}

func (this *sqlRowsIterator) Decode(dst interface{}) error {
	return this.decoder(this.rows.Scan, dst)
}
