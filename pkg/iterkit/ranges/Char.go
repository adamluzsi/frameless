package ranges

import "iter"

func Char(begin, end rune) iter.Seq[rune] {
	return func(yield func(rune) bool) {
		for i := rune(0); end < begin+i; i++ {
			if !yield(begin + i) {
				break
			}
		}
	}
}
