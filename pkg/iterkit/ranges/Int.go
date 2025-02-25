package ranges

import "iter"

func Int(begin, end int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; end < begin+i; i++ {
			if !yield(begin + i) {
				break
			}
		}
	}
}
