package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/frameless/pkg/datastruct/datastructcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func ExampleOrderedSet() {
	var set datastruct.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.ToSlice() // []string{"foo", "bar", "baz"}
	set.Len()     // 3
}

func ExampleOrderedSet_fromSlice() {
	var vs = []string{"foo", "bar", "baz", "foo"}
	var set = datastruct.OrderedSet[string]{}.FromSlice(vs)
	set.ToSlice() // []string{"foo", "bar", "baz"}
	set.Len()     // 3
}

func ExampleOrderedSet_iterate() {
	var set datastruct.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")

	for v := range set.Iter() {
		_ = v // "foo" -> "bar" -> "baz"
	}
}

func ExampleOrderedSet_has() {
	var set datastruct.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.Has("foo") // true
	set.Has("bar") // true
	set.Has("oof") // false
}

func TestOrderedSet(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("Add and Has", func(t *testing.T) {
		var (
			set      datastruct.OrderedSet[int]
			value    = rnd.Int()
			othValue = random.Unique(rnd.Int, value)
		)

		// Initially, the set should not contain the random value
		assert.False(t, set.Has(value))

		// After adding the value, Has should return true
		set.Append(value)
		assert.True(t, set.Has(value))
		assert.False(t, set.Has(othValue))
	})

	t.Run("FromSlice", func(t *testing.T) {
		values := []int{rnd.Int(), rnd.Int()}
		set := datastruct.OrderedSet[int]{}.FromSlice(values)

		for _, v := range values {
			assert.True(t, set.Has(v), "Set should contain the value added from the slice")
		}
	})

	t.Run("ToSlice uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := datastruct.OrderedSet[int]{}.FromSlice(exp)
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

	t.Run("FromSlice uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := datastruct.OrderedSet[int]{}.FromSlice(exp)
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
		set := datastruct.OrderedSet[int]{}.FromSlice(exp)
		got := set.ToSlice()

		assert.Equal(t, exp, got, "values were expected, and in the same order")
	})

	vs := testcase.Var[[]string]{
		ID: "already used values",
		Init: func(t *testcase.T) []string {
			return []string{}
		},
	}
	c := datastructcontract.ListConfig[string]{
		MakeT: func(tb testing.TB) string {
			t := tb.(*testcase.T)
			// we produce unique values because a Set type only accept new unique value
			v := random.Unique(func() string { return t.Random.String() }, vs.Get(t)...)
			testcase.Append(t, vs, v)
			return v
		},
	}

	t.Run("implements List", datastructcontract.List(func(tb testing.TB) datastruct.List[string] {
		return &datastruct.OrderedSet[string]{}
	}, c).Test)

	t.Run("implements ordered List", datastructcontract.OrderedList(func(tb testing.TB) datastruct.List[string] {
		return &datastruct.OrderedSet[string]{}
	}, c).Test)
}
