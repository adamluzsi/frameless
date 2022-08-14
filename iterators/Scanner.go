package iterators

import (
	"bufio"
	"io"
)

func Scanner[T string | []byte](s *bufio.Scanner, closer io.Closer) *ScannerIter[T] {
	return &ScannerIter[T]{
		Scanner: s,
		Closer:  closer,
	}
}

type ScannerIter[T string | []byte] struct {
	*bufio.Scanner
	Closer io.Closer
	value  T
}

func (i *ScannerIter[T]) Next() bool {
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

func (i *ScannerIter[T]) Err() error {
	return i.Scanner.Err()
}

func (i *ScannerIter[T]) Close() error {
	if i.Closer == nil {
		return nil
	}
	return i.Closer.Close()
}

func (i *ScannerIter[T]) Value() T {
	return i.value
}
