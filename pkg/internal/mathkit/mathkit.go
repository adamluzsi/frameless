package mathkit

import (
	"math"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/must"
)

type Int interface {
	int | int8 | int16 | int32 | int64
}

type UInt interface {
	uint | uint8 | uint16 | uint32 | uint64
}

type Float interface {
	float32 | float64
}

type Number interface {
	Int | UInt | Float
}

func MaxInt[INT Int]() INT {
	var v int64
	switch any(*new(INT)).(type) {
	case int:
		v = math.MaxInt
	case int8:
		v = math.MaxInt8
	case int16:
		v = math.MaxInt16
	case int32:
		v = math.MaxInt32
	case int64:
		v = math.MaxInt64
	default:
		panic("not-implemented")
	}
	return INT(v)
}

func MinInt[INT Int]() INT {
	var v int64
	switch any(*new(INT)).(type) {
	case int:
		v = math.MinInt
	case int8:
		v = math.MinInt8
	case int16:
		v = math.MinInt16
	case int32:
		v = math.MinInt32
	case int64:
		v = math.MinInt64
	default:
		panic("not-implemented")
	}
	return INT(v)
}

const ErrOverflow errorkit.Error = "ErrOverflow"

func Sum[int Int](a, b int) (int, error) {
	if CanSumOverflow(a, b) {
		var zero int
		return zero, ErrOverflow.F("%T overflow", a)
	}
	return a + b, nil
}

func MustSum[int Int](a, b int) int {
	return must.Must(Sum[int](a, b))
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

func CanSumOverflow[int Int](a, b int) bool {
	less, more := a, b
	if more < less {
		less, more = more, less
	}
	var (
		max = MaxInt[int]()
		min = MinInt[int]()
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
