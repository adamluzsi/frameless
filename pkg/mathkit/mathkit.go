package mathkit

import (
	"cmp"
	"iter"
	"math/big"
	"unsafe"

	"go.llib.dev/frameless/internal/constraints"
	"go.llib.dev/frameless/pkg/errorkit"
)

type (
	Int    constraints.Int
	UInt   constraints.UInt
	Float  constraints.Float
	Number constraints.Number
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

func SumInt[INT Int](a, b INT) (INT, bool) {
	if CanIntSumOverflow(a, b) {
		var zero INT
		return zero, false
	}
	return a + b, true
}

func CanIntSumOverflow[INT Int](a, b INT) bool {
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
	if x == 0 || y == 0 {
		return false
	}
	var max AInt
	if isMulResPositive(x, y) {
		max = AbsInt(MaxInt[INT]())
	} else {
		max = AbsInt(MinInt[INT]())
	}
	var maxMul = max / AbsInt(x)
	return maxMul < AbsInt(y)
}

func isMulResPositive[INT Int](x, y INT) bool {
	switch {
	case x < 0 && 0 < y: // - * + == -
		return false
	case 0 < x && y < 0: // + * - == -
		return false
	default:
		return true
	}
}

type AInt = uint64

func AbsInt[N Int](n N) AInt {
	if n == MinInt[N]() {
		// make it overflow into MaxInt
		// and then add +1 to be equal with the Abs MinInt
		return AInt(n-1) + 1
	}
	if n < 0 {
		n = -n
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

func (i BigInt[INT]) FromBigInt(n *big.Int) BigInt[INT] {
	return BigInt[INT]{i: n}.tryToSwitchToIntMode()
}

func (i BigInt[INT]) String() string {
	return i.switchToBigIntMode().i.String()
}

func (i BigInt[INT]) ToInt() (INT, bool) {
	i = i.tryToSwitchToIntMode()
	return i.n, i.i == nil
}

func (i BigInt[INT]) ToBigInt() *big.Int {
	return i.switchToBigIntMode().i
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
		return cmp.Compare(i.n, o.n)
	}
	return i.switchToBigIntMode().i.Cmp(o.switchToBigIntMode().i)
}

// Iter produces non-zero integer values ranging from 1 to the maximum or minimum possible integer value.
// When you add up all these yielded values, they equal the current big integer being iterated over.
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
	var isNegative = cmp < 0
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
			if cursor.Compare(UpperBoundary) <= 0 {
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
	if i.i == nil && n.i == nil && !CanIntSumOverflow(i.n, n.n) {
		return BigInt[INT]{n: i.n + n.n}
	}
	return i.on(i, n, func(base, x, y *big.Int) *big.Int {
		return base.Add(x, y)
	})
}

func (i BigInt[INT]) Sub(n BigInt[INT]) BigInt[INT] {
	if i.i == nil && n.i == nil && !CanIntSumOverflow(i.n, -n.n) {
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

func (i BigInt[INT]) IsZero() bool {
	return i.Compare(BigInt[INT]{}) == 0
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

	if cmp == 0 {
		return zero
	}

	var toIntMode = func() BigInt[INT] {
		return BigInt[INT]{n: INT(i.i.Int64())}
	}

	if cmp < 0 && 0 <= i.Compare(BigInt[INT]{n: MinInt[INT]()}) {
		return toIntMode()
	}

	if 0 < cmp && i.Compare(BigInt[INT]{n: MaxInt[INT]()}) <= 0 {
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
