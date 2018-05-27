package iterate

import (
	"bufio"
	"fmt"
	"io"

	"github.com/adamluzsi/frameless"
)

type textIterator struct {
	scanner *bufio.Scanner
	io      io.ReadCloser
}

func (this *textIterator) More() bool {
	return this.scanner.Scan()
}

func (this *textIterator) Err() error {
	return this.scanner.Err()
}

func (this *textIterator) Close() error {
	return this.io.Close()
}

func (this *textIterator) Decode(container interface{}) error {
	switch v := container.(type) {
	case *string:
		*(container).(*string) = this.scanner.Text()

	default:
		panic(fmt.Sprintf("unknown type %s\n", v))
	}

	return this.scanner.Err()
}

func LineByLine(rc io.ReadCloser) frameless.Iterator {
	return &textIterator{scanner: bufio.NewScanner(rc), io: rc}
}
