package iterate

import (
	"bufio"
	"fmt"
	"io"

	"github.com/adamluzsi/frameless/dataproviders"
)

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

func LineByLine(r io.Reader) dataproviders.Iterator {
	s := bufio.NewScanner(r)

	return &textIterator{s}
}
