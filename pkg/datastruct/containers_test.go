package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestStack(t *testing.T) {
	t.Run("on nil stack", func(t *testing.T) {
		expected := random.New(random.CryptoSeed{}).Int()
		var stack datastruct.Stack[int]
		_, ok := stack.Last()
		assert.False(t, ok)
		assert.True(t, stack.IsEmpty())
		_, ok = stack.Pop()
		assert.False(t, ok)
		assert.True(t, stack.IsEmpty())
		stack.Push(expected)
		assert.False(t, stack.IsEmpty())
		got, ok := stack.Last()
		assert.True(t, ok)
		assert.Equal(t, expected, got)
		assert.False(t, stack.IsEmpty())
		got, ok = stack.Pop()
		assert.True(t, ok)
		assert.Equal(t, expected, got)
		assert.True(t, stack.IsEmpty())
	})
	t.Run("on empty stack", func(t *testing.T) {
		expected := random.New(random.CryptoSeed{}).Int()
		stack := datastruct.Stack[int]{}
		_, ok := stack.Last()
		assert.False(t, ok)
		assert.True(t, stack.IsEmpty())
		_, ok = stack.Pop()
		assert.False(t, ok)
		assert.True(t, stack.IsEmpty())
		stack.Push(expected)
		assert.False(t, stack.IsEmpty())
		got, ok := stack.Last()
		assert.True(t, ok)
		assert.Equal(t, expected, got)
		assert.False(t, stack.IsEmpty())
		got, ok = stack.Pop()
		assert.True(t, ok)
		assert.Equal(t, expected, got)
		assert.True(t, stack.IsEmpty())
	})
}

func TestSet(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("Add and Has", func(t *testing.T) {
		var (
			set      datastruct.Set[int]
			value    = rnd.Int()
			othValue = random.Unique(rnd.Int, value)
		)

		// Initially, the set should not contain the random value
		assert.False(t, set.Has(value))

		// After adding the value, Has should return true
		set.Add(value)
		assert.True(t, set.Has(value))
		assert.False(t, set.Has(othValue))
	})

	t.Run("MakeSet from slice", func(t *testing.T) {
		values := []int{rnd.Int(), rnd.Int()}
		set := datastruct.MakeSet(values...)

		for _, v := range values {
			assert.True(t, set.Has(v), "Set should contain the value added from the slice")
		}
	})

	t.Run("ToSlice uniqueness", func(t *testing.T) {
		values := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := datastruct.MakeSet(values...)
		slice := set.ToSlice()

		// Create a temporary map to check for duplicates in the slice
		tempMap := make(map[int]struct{})
		for _, item := range slice {
			if _, exists := tempMap[item]; exists {
				t.Errorf("Duplicate found in the slice returned by ToSlice, which should not happen")
			}
			tempMap[item] = struct{}{}
		}
		// Ensure all original unique values are present
		for _, v := range values {
			_, ok := tempMap[v]
			assert.True(t, ok, "All unique values from the initial slice should be present in the slice returned by ToSlice")
		}
	})
}
