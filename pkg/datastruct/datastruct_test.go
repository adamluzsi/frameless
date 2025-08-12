package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/testcase"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
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

	t.Run("FromSlice", func(t *testing.T) {
		values := []int{rnd.Int(), rnd.Int()}
		set := datastruct.Set[int]{}.FromSlice(values)

		for _, v := range values {
			assert.True(t, set.Has(v), "Set should contain the value added from the slice")
		}
	})

	t.Run("ToSlice uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := datastruct.Set[int]{}.FromSlice(exp)
		got := set.ToSlice()

		// Create a temporary map to check for duplicates in the slice
		tempMap := make(map[int]struct{})
		for _, item := range got {
			if _, exists := tempMap[item]; exists {
				t.Errorf("Duplicate found in the slice returned by ToSlice, which should not happen")
			}
			tempMap[item] = struct{}{}
		}
		// Ensure all original unique values are present
		for _, v := range exp {
			_, ok := tempMap[v]

			assert.True(t, ok, assert.MessageF("%v was missing", v),
				"\nAll unique values from the initial slice should be present in the slice returned by ToSlice.")
		}
	})

	t.Run("FromValues uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := datastruct.Set[int]{}.From(exp...)
		got := set.ToSlice()

		// Create a temporary map to check for duplicates in the slice
		tempMap := make(map[int]struct{})
		for _, item := range got {
			if _, exists := tempMap[item]; exists {
				t.Errorf("Duplicate found in the slice returned by ToSlice, which should not happen")
			}
			tempMap[item] = struct{}{}
		}
		// Ensure all original unique values are present
		for _, v := range exp {
			_, ok := tempMap[v]

			assert.True(t, ok, assert.MessageF("%v was missing", v),
				"\nAll unique values from the initial slice should be present in the slice returned by ToSlice.")
		}
	})

	t.Run("ToSlice is ordered by default", func(t *testing.T) {
		exp := []int{1, 5, 2, 7, 3, 9} // Intentional duplicate to test uniqueness
		set := datastruct.Set[int]{}.FromSlice(exp)
		got := set.ToSlice()

		assert.Equal(t, exp, got, "values were expected, and in the same order")
	})
}

var _ datastruct.MapInterface[string, int] = datastruct.Map[string, int]{}

func TestMapAdd(t *testing.T) {
	var (
		key string = "foo"
		v1  int    = 1
		v2  int    = 2
		v3  int    = 3
	)

	var m = datastruct.Map[string, int]{}

	td1 := datastruct.MapAdd(m, key, v1)
	assert.Equal(t, m.Get(key), v1)

	td2 := datastruct.MapAdd(m, key, v2)
	assert.Equal(t, m.Get(key), v2)

	td3 := datastruct.MapAdd(m, key, v3)
	assert.Equal(t, m.Get(key), v3)

	td3()
	assert.Equal(t, m.Get(key), v2)

	td2()
	assert.Equal(t, m.Get(key), v1)

	td1()
	_, ok := m.Lookup(key)
	assert.False(t, ok)
}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)

	m := let.Var(s, func(t *testcase.T) datastruct.Map[string, int] {
		return datastruct.Map[string, int]{}
	})

	s.Test("smoke", func(t *testcase.T) {
		var (
			key  string = t.Random.String()
			val1 int    = t.Random.Int()
			val2 int    = t.Random.Int()
		)

		_, ok := m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, m.Get(t).Keys())
		assert.Equal(t, 0, m.Get(t).Len())

		m.Get(t).Set(key, val1)
		got, ok := m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val1, got)
		assert.Equal(t, val1, m.Get(t).Get(key))
		assert.Contains(t, m.Get(t).Keys(), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Set(key, val2)
		got, ok = m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val2, got)
		assert.Equal(t, val2, m.Get(t).Get(key))
		assert.Contains(t, m.Get(t).Keys(), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Delete(key)
		_, ok = m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, m.Get(t).Keys())
		assert.Equal(t, 0, m.Get(t).Len())
	})
}
