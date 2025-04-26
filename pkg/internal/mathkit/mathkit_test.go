package mathkit_test

import (
	"math"
	"testing"

	"go.llib.dev/frameless/pkg/internal/mathkit"
	"go.llib.dev/testcase/assert"
)

func TestMaxIntMultiplier(t *testing.T) {
	t.Run("Positive small integer", func(t *testing.T) {
		input := 2
		expected := math.MaxInt / input
		result := mathkit.MaxIntMultiplier(input)
		assert.Equal(t, expected, result)
	})

	t.Run("MaxInt as input", func(t *testing.T) {
		input := math.MaxInt
		expected := 1
		result := mathkit.MaxIntMultiplier(input)
		assert.Equal(t, expected, result)
	})

	t.Run("Negative input", func(t *testing.T) {
		input := -3
		expected := math.MaxInt / input
		result := mathkit.MaxIntMultiplier(input)
		assert.Equal(t, expected, result)
	})

	t.Run("Zero input", func(t *testing.T) {
		input := 0
		expected := math.MaxInt
		result := mathkit.MaxIntMultiplier(input)
		assert.Equal(t, expected, result)
	})

	t.Run("Just below overflow", func(t *testing.T) {
		input := 5
		maxMul := mathkit.MaxIntMultiplier(input)
		result := input * maxMul
		assert.True(t, result <= math.MaxInt)
	})

	t.Run("Just above overflow", func(t *testing.T) {
		input := 5
		maxMul := mathkit.MaxIntMultiplier(input)
		overflowCandidate := input * (maxMul + 1)
		// Should overflow an int if done naively; we check if it's less than zero or wraps
		assert.True(t, overflowCandidate < 0 || overflowCandidate < input, "Expected overflow")
	})
}
