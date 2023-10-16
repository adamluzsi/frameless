package ranges

import "go.llib.dev/frameless/ports/iterators"

func Char(begin, end rune) iterators.Iterator[rune] {
	return &charRange{Begin: begin, End: end}
}

type charRange struct {
	Begin, End rune
	nextIndex  rune
	closed     bool
}

func (rr *charRange) Close() error {
	rr.closed = true
	return nil
}

func (rr *charRange) Err() error {
	return nil
}

func (rr *charRange) Next() bool {
	if rr.closed {
		return false
	}
	if rr.End < rr.Begin+rr.nextIndex {
		return false
	}
	rr.nextIndex++
	return true
}

func (rr *charRange) Value() rune {
	return rr.Begin + rr.nextIndex - 1
}
