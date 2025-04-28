package mathkit

import (
	"math/big"
	"unsafe"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/internal/constraints"
	"go.llib.dev/frameless/pkg/must"
)

type (
	Int    = constraints.Int
	UInt   = constraints.UInt
	Float  = constraints.Float
	Number = constraints.Number
)

func MaxInt[T Int]() T {
	var zero T
	// Get the size in bits by multiplying byte size by 8
	typeSizeInBits := 8 * unsafe.Sizeof(zero)
	// Maximum value is 2^(n-1) - 1 where n is the number of bits
	// For signed integers, this is all bits set except the sign bit
	return T((1 << (typeSizeInBits - 1)) - 1)
}

func MinInt[T Int]() T {
	var zero T
	typeSizeInBits := 8 * unsafe.Sizeof(zero)
	// Minimum value is -2^(n-1) for signed integers
	return T(-1 << (typeSizeInBits - 1))
}

const ErrOverflow errorkit.Error = "ErrOverflow"

func Sum[int Int](a, b int) (int, error) {
	if CanSumOverflow(a, b) {
		var zero int
		return zero, ErrOverflow.F("%T overflow", a)
	}
	return a + b, nil
}

func MustSum[INT Int](a, b INT) INT {
	return must.Must(Sum[INT](a, b))
}

// func SumR[int Int](a, b int) (int, int) {
// 	if b < a {
// 		a, b = b, a // less -> more
// 	}
// 	if CanSumOverflow(a, b) {
//
// 	}
// 	return a + b, 0
// }

func GuardSumF[int Int](x, y int, format string, a ...any) {
	if CanSumOverflow(x, y) {
		panic(ErrOverflow.F(format, a...))
	}
}

func CanSumOverflow[INT Int](a, b INT) bool {
	less, more := a, b
	if more < less {
		less, more = more, less
	}
	var (
		max = MaxInt[INT]()
		min = MinInt[INT]()
	)
	switch {
	case 0 < less && 0 < more:
		// MinInt - -number -> MinInt plus abs less
		maxLess := max - more
		return maxLess < less // positive overflow
	case less < 0 && more < 0:
		// MinInt - -number -> MinInt plus abs less
		minMore := min - less
		return more < minMore // negative overflow
	case less < 0 && 0 < more:
		// there is no combination where a + b can cause overflow
		// because even MinInt plus MaxInt would only end up in zero.
	}
	return false //, 0, more + minMore
}

func MaxIntMultiplier[INT Int](x INT) INT {
	var maxInt = MaxInt[INT]()
	if x == 0 {
		return maxInt // no risk of overflow
	}
	return maxInt / x
}

func MinIntDivider[INT Int](x INT) INT {
	var minInt = MinInt[INT]()
	if x == 0 {
		return minInt // division by zero is invalid
	}
	return minInt / x
}

type BigInt[INT Int] struct {
	i *big.Int
	n INT
}

func (i BigInt[INT]) maxInt() *big.Int {
	return big.NewInt(int64(MaxInt[INT]()))
}

func (i BigInt[INT]) switchToBigIntMode() BigInt[INT] {
	if i.i != nil {
		return i
	}
	return BigInt[INT]{i: big.NewInt(int64(i.n))}
}

func (i BigInt[INT]) tryToSwitchToIntMode() BigInt[INT] {
	if i.i == nil {
		return i
	}
	if compare.IsLessOrEqual(i.i.Cmp(i.maxInt())) {
		return BigInt[INT]{n: INT(i.i.Int64())}
	}
	return i
}

func (i BigInt[INT]) Compare(o BigInt[INT]) int {
	if i.i == nil && o.i == nil { // fast path
		return compare.Numbers(i.n, o.n)
	}

	return i.switchToBigIntMode().i.Cmp(o.switchToBigIntMode().i)
}

func (i BigInt[INT]) Sum(n INT) BigInt[INT] {
	if i.i != nil {
		i.i = i.i.Add(i.i, big.NewInt(int64(n)))
		return i.tryToSwitchToIntMode()
	}
	if CanSumOverflow(i.n, n) {
		return i.switchToBigIntMode().Sum(n)
	}
	i.n += n
	return i
}

func (i BigInt[INT]) Sub(n INT) BigInt[INT] {
	if i.i != nil {
		i.i = i.i.Sub(i.i, big.NewInt(int64(n)))
		return i.tryToSwitchToIntMode()
	}
	if CanSumOverflow(i.n, -n) {
		return i.switchToBigIntMode().Sub(n)
	}
	i.n -= n
	return i
}

func (i BigInt[INT]) Mul(n INT) BigInt[INT] {
	if i.i != nil {
		i.i = i.i.Mul(i.i, big.NewInt(int64(n)))
		return i.tryToSwitchToIntMode()
	}
	if maxMul := MaxIntMultiplier[INT](i.n); maxMul < n {
		return i.switchToBigIntMode().Mul(n)
	}
	i.n *= n
	return i
}

func (i BigInt[INT]) Div(n INT) BigInt[INT] {
	if i.i != nil {
		i.i = i.i.Div(i.i, big.NewInt(int64(n)))
		return i.tryToSwitchToIntMode()
	}
	if minDiv := MinIntDivider[INT](i.n); n < minDiv {
		return i.switchToBigIntMode().Div(n)
	}
	i.n /= n
	return i
}
