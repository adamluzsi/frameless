package ranges

import "go.llib.dev/frameless/ports/iterators"

func Int(begin, end int) iterators.Iterator[int] {
	return &intRange{Begin: begin, End: end}
}

type intRange struct {
	Begin, End int
	nextIndex  int
	closed     bool
}

func (ir *intRange) Close() error {
	ir.closed = true
	return nil
}

func (ir *intRange) Err() error {
	return nil
}

func (ir *intRange) Next() bool {
	if ir.closed {
		return false
	}
	if ir.End < ir.Begin+ir.nextIndex {
		return false
	}
	ir.nextIndex++
	return true
}

func (ir *intRange) Value() int {
	return ir.Begin + ir.nextIndex - 1
}
