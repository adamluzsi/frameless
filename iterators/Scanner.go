package iterators

import (
	"bufio"
	"fmt"
	"io"
)

func NewScanner(rc io.Reader) *Scanner {
	return &Scanner{
		Scanner: bufio.NewScanner(rc),
		reader:  rc,
	}
}

type Scanner struct {
	*bufio.Scanner
	reader io.Reader
}

func (i *Scanner) Next() bool {
	return i.Scanner.Scan()
}

func (i *Scanner) Err() error {
	return i.Scanner.Err()
}

func (i *Scanner) Close() error {
	rc, ok := i.reader.(io.ReadCloser)

	if !ok {
		return nil
	}

	return rc.Close()
}

func (i *Scanner) Decode(container interface{}) error {
	switch v := container.(type) {
	case *string:
		*(container).(*string) = i.Scanner.Text()

	default:
		panic(fmt.Sprintf("unknown type %s\n", v))
	}

	return i.Scanner.Err()
}
