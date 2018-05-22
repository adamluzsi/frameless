package payload

import (
	"bufio"
	"fmt"
	"io"
	"net/http"

	"github.com/adamluzsi/frameless/dataprovider"
)

type PlainText struct {
	request *http.Request
	scanner *bufio.Scanner

	lastValue []byte
}

type textIterator struct {
	scanner *bufio.Scanner
}

func (ti *textIterator) More() bool {
	return ti.scanner.Scan()
}

func (ti *textIterator) Decode(container interface{}) error {
	switch v := container.(type) {
	case *string:
		*(container).(*string) = ti.scanner.Text()

	default:
		panic(fmt.Sprintf("unknown type %s\n", v))
	}

	return ti.scanner.Err()
}

func LineByLine(r io.Reader) dataprovider.Iterator {
	s := bufio.NewScanner(r)

	return &textIterator{s}
}
