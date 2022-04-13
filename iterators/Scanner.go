package iterators

import (
	"bufio"
	"io"
)

func NewScanner[T string | []byte](rc io.Reader) *Scanner[T] {
	return &Scanner[T]{
		Scanner: bufio.NewScanner(rc),
		Reader:  rc,
	}
}

type Scanner[T string | []byte] struct {
	*bufio.Scanner
	Reader io.Reader

	value T
}

func (i *Scanner[T]) Next() bool {
	if i.Scanner.Err() != nil {
		return false
	}
	if !i.Scanner.Scan() {
		return false
	}
	var v T
	var iface interface{} = v
	switch iface.(type) {
	case string:
		i.value = T(i.Scanner.Text())
	case []byte:
		i.value = T(i.Scanner.Bytes())
	}
	return true
}

func (i *Scanner[T]) Err() error {
	return i.Scanner.Err()
}

func (i *Scanner[T]) Close() error {
	rc, ok := i.Reader.(io.ReadCloser)
	if !ok {
		return nil
	}

	return rc.Close()
}

func (i *Scanner[T]) Value() T {
	return i.value
}
