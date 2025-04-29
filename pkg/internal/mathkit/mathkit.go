package mathkit

import (
	"iter"
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
	if x == 0 {
		return false
	}
	var (
		maxInt = MaxInt[INT]()
		maxMul = maxInt / Abs(x)
	)
	return maxMul < Abs(y)
}

func Abs[N Number](n N) N {
	if n < 0 {
		return -n
	}
	return n
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
	switch cmp {
	case 1:
		var (
			cursor        = i
			maxInt        = MaxInt[INT]()
			UpperBoundary = BigInt[INT]{n: maxInt}.switchToBigIntMode()
		)
		return func(yield func(INT) bool) {
			for {
				cmp := cursor.Compare(UpperBoundary)
				if compare.IsLessOrEqual(cmp) {
					last := cursor.tryToSwitchToIntMode()
					yield(last.n)
					return
				}
				if !yield(maxInt) {
					return
				}
				cursor = cursor.Sub(UpperBoundary)
			}
		}
	case -1:
		var (
			cursor        = i
			minInt        = MinInt[INT]()
			LowerBoundary = BigInt[INT]{n: minInt}.switchToBigIntMode()
		)
		return func(yield func(INT) bool) {
			for {
				cmp := cursor.Compare(LowerBoundary)
				if compare.IsGreaterOrEqual(cmp) {
					last := cursor.tryToSwitchToIntMode()
					yield(last.n)
					return
				}
				if !yield(minInt) {
					return
				}
				cursor = cursor.Sub(LowerBoundary) // -x - -y => -x + y
			}
		}
	default:
		panic("not-implemented")
	}
}

func (i BigInt[INT]) Abs() BigInt[INT] {
	if i.i == nil {
		return BigInt[INT]{n: Abs(i.n)}
	}
	return BigInt[INT]{i: (&big.Int{}).Abs(i.i)}
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
