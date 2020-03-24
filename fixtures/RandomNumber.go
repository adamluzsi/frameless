package fixtures

import (
	"math/rand"
	"sort"
)

// Intn returns, as an int, a non-negative pseudo-random number in [0,n).
// It panics if n <= 0.
func RandomIntn(n int) int {
	return rnd.Intn(n)
}

// RandomIntByRange returns, as an int, a non-negative pseudo-random number based on the received int range's [min,max).
// It panics if n <= 0.
func RandomIntByRange(intRange ...int) int {
	sort.Ints(intRange)
	from := intRange[0]
	to := intRange[len(intRange)-1]
	if from == to {
		return from
	}
	return from + rand.Intn(to-from)
}
