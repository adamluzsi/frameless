package sqlrows

import "io"

type Scan func(...interface{}) error

type Decoder func(scan Scan, destination interface{}) error

type Rows interface {
	io.Closer
	Next() bool
	Err() error
	Scan(...interface{}) error
}
