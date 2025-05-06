package mathkit

import (
	"iter"
	"math/big"
	"unsafe"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/internal/constraints"
	"go.llib.dev/testcase/pp"
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

func Sum[INT Int](a, b INT) (INT, bool) {
	if CanSumOverflow(a, b) {
		var zero INT
		return zero, false
	}
	return a + b, true
}

func CanSumOverflow[INT Int](a, b INT) bool {
	less, more := a, b
	if more < less {
		less, more = more, less
	}
	switch {
	case 0 < less && 0 < more:
		var max = MaxInt[INT]()
		// MinInt - -number -> MinInt plus abs less
		maxLess := max - more
		return maxLess < less // positive overflow
	case less < 0 && more < 0:
		var min = MinInt[INT]()
		// MinInt - -number -> MinInt plus abs less
		minMore := min - less // min - -less -> min + abs(less)
		return more < minMore // negative overflow
	case less < 0 && 0 < more:
		// there is no combination where a + b can cause overflow
		// because even MinInt plus MaxInt would only end up in zero.
	}
	return false //, 0, more + minMore
}

func CanIntMulOverflow[INT Int](x, y INT) bool {
	return CanIntMulOverflowX(x, y)
}

func CanIntMulOverflowX[INT Int](x, y INT) bool {
	pp.PP(x, y)
	if x == 0 || y == 0 {
		return false
	}
	var (
		maxInt = AbsInt(MaxInt[INT]())
		maxMul = maxInt / AbsInt(x)
	)
	return maxMul < AbsInt(y)
}

func CanIntMulOverflowY[INT Int](x, y INT) bool {
	if x == 0 || y == 0 ||
		x == 1 || y == 1 ||
		x == -1 || y == -1 {
		return false
	}
	var (
		o  = x * y
		ok bool
	)
	pp.PP(x, y, o)
	switch {
	case 0 < x && 0 < y: // + * + = +
		ok = x < o && y < o
	case x < 0 && y < 0: // - * - = +
		ok = x < o && y < o
	case x < 0 && 0 < y: // - * + = -
		ok = o < x && o < y
	case 0 < x && y < 0: // + * - = -
		ok = o < x && o < y
	default:
		panic("not-implemented")
	}
	return !ok
}

// AInt is an absolut int value type.
// It is an alias type to remove unnecessary
//
// It is kept always on the maximums supported uint type,
// which is currently uint64.
type AInt = uint64

func AbsInt[N Int](n N) AInt {
	if n < 0 {
		return AInt(n)
	}
	return AInt(n)
}

type BigInt[INT Int] struct {
	i *big.Int
	n INT
}

func (i BigInt[INT]) Of(n INT) BigInt[INT] {
	return BigInt[INT]{n: n}
}

func (i BigInt[INT]) String() string {
	return i.switchToBigIntMode().i.String()
}

func (i BigInt[INT]) ToInt() (INT, bool) {
	i = i.tryToSwitchToIntMode()
	return i.n, i.i == nil
}

const ErrParseBigInt errorkit.Error = "ErrParseBigInt"

func (BigInt[INT]) Parse(raw string) (BigInt[INT], error) {
	i, ok := big.NewInt(0).SetString(raw, 10)
	if !ok {
		return BigInt[INT]{}, ErrParseBigInt.F("unable to parse %q", raw)
	}
	return BigInt[INT]{i: i}.tryToSwitchToIntMode(), nil
}

func (i BigInt[INT]) Compare(o BigInt[INT]) int {
	if i.i == nil && o.i == nil { // fast path
		return compare.Numbers(i.n, o.n)
	}
	return i.switchToBigIntMode().i.Cmp(o.switchToBigIntMode().i)
}

func (i BigInt[INT]) Iter() iter.Seq[INT] {
	var cmp = i.Compare(BigInt[INT]{}.Of(0))
	if cmp == 0 {
		return func(yield func(INT) bool) {}
	}
	if i.i == nil { // not big int mode
		return func(yield func(INT) bool) {
			yield(i.n)
		}
	}
	var isNegative = compare.IsLess(cmp)
	var format = func(n INT) INT {
		if isNegative {
			return -n
		}
		return n
	}
	var (
		maxInt        = MaxInt[INT]() // MaxInt is perfect because a negative MaxInt is also a valid value, unlike a positive MinInt
		UpperBoundary = BigInt[INT]{n: maxInt}.switchToBigIntMode()
	)
	return func(yield func(INT) bool) {
		var cursor = i.Abs()
		for {
			cmp := cursor.Compare(UpperBoundary)
			if compare.IsLessOrEqual(cmp) {
				last := cursor.tryToSwitchToIntMode()
				yield(format(last.n))
				return
			}
			if !yield(format(maxInt)) {
				return
			}
			cursor = cursor.Sub(UpperBoundary)
		}
	}
}

func (i BigInt[INT]) Abs() BigInt[INT] {
	if i.i == nil {
		if 0 <= i.n {
			return i
		}
		if MinInt[INT]() < i.n {
			return BigInt[INT]{n: -i.n}
		}
	}
	i = i.switchToBigIntMode()
	i.i = big.NewInt(0).Abs(i.i)
	return i.tryToSwitchToIntMode()
}

func (i BigInt[INT]) Add(n BigInt[INT]) BigInt[INT] {
	if i.i == nil && n.i == nil && !CanSumOverflow(i.n, n.n) {
		return BigInt[INT]{n: i.n + n.n}
	}
	return i.on(i, n, func(base, x, y *big.Int) *big.Int {
		return base.Add(x, y)
	})
}

func (i BigInt[INT]) Sub(n BigInt[INT]) BigInt[INT] {
	if i.i == nil && n.i == nil && !CanSumOverflow(i.n, -n.n) {
		return BigInt[INT]{n: i.n - n.n}
	}
	return i.on(i, n, func(base, x, y *big.Int) *big.Int {
		return base.Sub(x, y)
	})
}

func (i BigInt[INT]) Mul(n BigInt[INT]) BigInt[INT] {
	if i.i == nil && n.i == nil && !CanIntMulOverflow(i.n, n.n) {
		return BigInt[INT]{n: i.n * n.n}
	}
	return i.on(i, n, func(base, x, y *big.Int) *big.Int {
		return base.Mul(x, y)
	})
}

func (i BigInt[INT]) Div(n BigInt[INT]) BigInt[INT] {
	if i.i == nil && n.i == nil {
		// it is not possible to have two int value DIV each other and cause an int overflow
		return BigInt[INT]{n: i.n / n.n}
	}
	return i.on(i, n, func(base, x, y *big.Int) *big.Int {
		return base.Div(x, y)
	})
}

// func (i BigInt[INT]) maxInt()  {
// 	return big.NewInt(int64(MaxInt[INT]()))
// }

func (i BigInt[INT]) minInt() *big.Int {
	return big.NewInt(int64(MinInt[INT]()))
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

	var zero BigInt[INT]
	cmp := i.Compare(zero)

	if compare.IsEqual(cmp) {
		return zero
	}

	var toIntMode = func() BigInt[INT] {
		return BigInt[INT]{n: INT(i.i.Int64())}
	}

	if compare.IsLess(cmp) && compare.IsGreaterOrEqual(i.Compare(BigInt[INT]{n: MinInt[INT]()})) {
		return toIntMode()
	}

	if compare.IsGreater(cmp) && compare.IsLessOrEqual(i.Compare(BigInt[INT]{n: MaxInt[INT]()})) {
		return toIntMode()
	}

	return i
}

func (i BigInt[INT]) on(x, y BigInt[INT], do func(base, x, y *big.Int) *big.Int) BigInt[INT] {
	base := &big.Int{}
	xBI := x.switchToBigIntMode().i
	yBI := y.switchToBigIntMode().i
	return BigInt[INT]{i: do(base, xBI, yBI)}.
		tryToSwitchToIntMode()
}
