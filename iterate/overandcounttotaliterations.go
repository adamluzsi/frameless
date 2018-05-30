package iterate

import (
	"github.com/adamluzsi/frameless"
)

// OverAndCountTotalIterations will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func OverAndCountTotalIterations(i frameless.Iterator) (int, error) {
	defer i.Close()

	total := 0

	for i.Next() {
		total++
	}

	return total, i.Err()
}
