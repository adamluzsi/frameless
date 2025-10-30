package predicate_test

import (
	"math/big"
	"time"

	"go.llib.dev/frameless/port/predicate"
)

var _ predicate.Comparable[time.Time] = (*time.Time)(nil)

var _ predicate.ComparableShort[*big.Int] = (*big.Int)(nil)
