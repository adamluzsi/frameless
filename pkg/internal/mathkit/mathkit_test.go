package mathkit_test

import (
	"math"
	"testing"

	"go.llib.dev/frameless/pkg/internal/mathkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/testcase/assert"
)

func TestAbs_smoke(t *testing.T) {
	assert.Equal[int](t, 42, mathkit.Abs[int](42))
	assert.Equal[int](t, 42, mathkit.Abs[int](-42))

	assert.Equal[int8](t, 2, mathkit.Abs[int8](2))
	assert.Equal[int8](t, 2, mathkit.Abs[int8](-2))

	assert.Equal[int16](t, 2, mathkit.Abs[int16](2))
	assert.Equal[int16](t, 2, mathkit.Abs[int16](-2))

	assert.Equal[int32](t, 2, mathkit.Abs[int32](2))
	assert.Equal[int32](t, 2, mathkit.Abs[int32](-2))

	assert.Equal[int64](t, 2, mathkit.Abs[int64](2))
	assert.Equal[int64](t, 2, mathkit.Abs[int64](-2))

	assert.Equal[float32](t, 2.1, mathkit.Abs[float32](2.1))
	assert.Equal[float32](t, 2.1, mathkit.Abs[float32](-2.1))
}

func TestMaxInt(t *testing.T) {
	t.Run("core types", func(t *testing.T) {
		assert.Equal(t, math.MaxInt8, mathkit.MaxInt[int8]())
		assert.Equal(t, math.MaxInt16, mathkit.MaxInt[int16]())
		assert.Equal(t, math.MaxInt32, mathkit.MaxInt[int32]())
		assert.Equal(t, math.MaxInt64, mathkit.MaxInt[int64]())
	})
	t.Run("type based on core types", func(t *testing.T) {
		type Int8 int8
		assert.Equal(t, math.MaxInt8, mathkit.MaxInt[Int8]())
		type Int16 int16
		assert.Equal(t, math.MaxInt16, mathkit.MaxInt[Int16]())
		type Int32 int32
		assert.Equal(t, math.MaxInt32, mathkit.MaxInt[Int32]())
		type Int64 int64
		assert.Equal(t, math.MaxInt64, mathkit.MaxInt[Int64]())
	})
}

func TestMinInt(t *testing.T) {
	t.Run("core types", func(t *testing.T) {
		assert.Equal(t, math.MinInt8, mathkit.MinInt[int8]())
		assert.Equal(t, math.MinInt16, mathkit.MinInt[int16]())
		assert.Equal(t, math.MinInt32, mathkit.MinInt[int32]())
		assert.Equal(t, math.MinInt64, mathkit.MinInt[int64]())
	})
	t.Run("type based on core types", func(t *testing.T) {
		type Int8 int8
		assert.Equal(t, math.MinInt8, mathkit.MinInt[Int8]())
		type Int16 int16
		assert.Equal(t, math.MinInt16, mathkit.MinInt[Int16]())
		type Int32 int32
		assert.Equal(t, math.MinInt32, mathkit.MinInt[Int32]())
		type Int64 int64
		assert.Equal(t, math.MinInt64, mathkit.MinInt[Int64]())
	})
}

// func TestMaxIntMultiplier(t *testing.T) {
// 	t.Run("Positive small integer", func(t *testing.T) {
// 		input := 2
// 		expected := math.MaxInt / input
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("MaxInt as input", func(t *testing.T) {
// 		input := math.MaxInt
// 		expected := 1
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Negative input", func(t *testing.T) {
// 		input := -3
// 		expected := math.MaxInt / input
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Zero input", func(t *testing.T) {
// 		input := 0
// 		expected := math.MaxInt
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Just below overflow", func(t *testing.T) {
// 		input := 5
// 		maxMul := mathkit.CanIntMulOverflow(input)
// 		result := input * maxMul
// 		assert.True(t, result <= math.MaxInt)
// 	})

// 	t.Run("Just above overflow", func(t *testing.T) {
// 		input := 5
// 		maxMul := mathkit.CanIntMulOverflow(input)
// 		overflowCandidate := input * (maxMul + 1)
// 		// Should overflow an int if done naively; we check if it's less than zero or wraps
// 		assert.True(t, overflowCandidate < 0 || overflowCandidate < input, "Expected overflow")
// 	})
// }

// func TestMinIntDivider(t *testing.T) {
// 	t.Run("Positive small integer", func(t *testing.T) {
// 		input := 2
// 		expected := math.MinInt / input
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("MinInt as input", func(t *testing.T) {
// 		input := math.MinInt
// 		expected := 1
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Negative input", func(t *testing.T) {
// 		input := -3
// 		expected := math.MinInt / input
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Zero input", func(t *testing.T) {
// 		input := 0
// 		expected := math.MinInt
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Just above minimum", func(t *testing.T) {
// 		input := 5
// 		minDiv := mathkit.MinIntDivider(input)
// 		result := minDiv / input
// 		assert.True(t, result >= math.MinInt)
// 	})

// 	t.Run("Below min divider produces larger quotient", func(t *testing.T) {
// 		input := 5
// 		minDiv := mathkit.MinIntDivider(input)
// 		smallerDiv := minDiv - 1

// 		// Division of a smaller numerator should produce a slightly larger result
// 		originalResult := minDiv / input
// 		newResult := smallerDiv / input

// 		assert.True(t, newResult >= originalResult, "Expected larger or equal quotient when decreasing numerator")
// 	})
// }

func Test_bigInt(t *testing.T) {
	t.Run("pos", func(t *testing.T) {
		n := mathkit.BigInt[int]{}
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MaxInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MaxInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(42))

		vs := iterkit.Collect(n.Iter())

		assert.Equal(t, vs, []int{
			math.MaxInt,
			math.MaxInt,
			42,
		})
	})
	t.Run("neg", func(t *testing.T) {
		n := mathkit.BigInt[int]{}
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MinInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MinInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(-42))

		vs := iterkit.Collect(n.Iter())

		assert.Equal(t, vs, []int{
			math.MinInt,
			math.MinInt,
			-42,
		})
	})
}

func TestBigInt(t *testing.T) {

}
