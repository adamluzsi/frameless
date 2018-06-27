package iterateover_test

import (
	"errors"
	"io"
	"reflect"
)

type ReadCloser struct {
	IsClosed bool
	io       io.Reader
}

func NewReadCloser(r io.Reader) *ReadCloser {
	return &ReadCloser{io: r, IsClosed: false}
}

func (this *ReadCloser) Read(p []byte) (n int, err error) {
	return this.io.Read(p)
}

func (this *ReadCloser) Close() error {
	if this.IsClosed {
		return errors.New("already closed")
	}

	this.IsClosed = true
	return nil
}

type Rows struct {
	IsClosed bool
	Error    error
	Rows     [][]string
	index    int
}

func NewRows(rows [][]string, Error error) *Rows {
	return &Rows{
		Rows:  rows,
		Error: Error,
		index: -1,
	}
}

func (this *Rows) Close() error {
	if this.IsClosed {
		return errors.New("already IsClosed")
	}

	this.IsClosed = true

	return nil
}

func (this *Rows) Err() error {
	return this.Error
}

func (this *Rows) Next() bool {
	this.index++

	return len(this.Rows) > this.index
}

func (this *Rows) Scan(dests ...interface{}) error {
	for i, d := range dests {
		dr := reflect.ValueOf(d)
		sr := reflect.ValueOf(this.Rows[this.index][i])
		dr.Elem().Set(sr)
	}

	return nil
}

type SQLRowsSubject struct {
	C1 string
	C2 string
	C3 string
}

type X struct{ V string }
