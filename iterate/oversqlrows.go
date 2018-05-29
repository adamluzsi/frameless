package iterate

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterate/oversqlrows"
)

func OverSQLRows(rows oversqlrows.Rows, decoder oversqlrows.Decoder) frameless.Iterator {
	return &sqlRowsIterator{rows: rows, decoder: decoder}
}

type sqlRowsIterator struct {
	rows    oversqlrows.Rows
	decoder oversqlrows.Decoder
}

func (this *sqlRowsIterator) Close() error {
	return this.rows.Close()
}

func (this *sqlRowsIterator) Err() error {
	return this.rows.Err()
}

func (this *sqlRowsIterator) More() bool {
	return this.rows.Next()
}

func (this *sqlRowsIterator) Decode(dest interface{}) error {
	return this.decoder(this.rows.Scan, dest)
}
