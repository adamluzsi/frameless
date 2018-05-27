package iterate_test

import (
	"errors"
	"reflect"
)

type Rows struct {
	closed bool
	Error  error
	Rows   [][]string
	index  int
}

func (this *Rows) Close() error {
	if this.closed {
		return errors.New("already closed")
	}

	this.closed = true

	return nil
}

func (this *Rows) Err() error {
	return this.Error
}

func (this *Rows) Next() bool {
	isNext := len(this.Rows) > this.index
	this.index++
	return isNext
}

func (this *Rows) Scan(dests ...interface{}) error {
	for i, d := range dests {
		dr := reflect.ValueOf(d)

		dr.Set(reflect.ValueOf(this.Rows[this.index][i]))
	}

	return nil
}
