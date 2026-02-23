package dsset_test

import (
	"testing"

	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/frameless/port/ds/dsset"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func ExampleSet() {
	var set dsset.Set[string]
	set.Append("foo", "bar", "baz")
	for v := range set.Values() {
		_ = v // "foo" / "bar" / "baz"
	}
}

func TestSet(t *testing.T) {
	s := testcase.NewSpec(t)

	vs := testcase.Var[[]string]{
		ID: "already used values",
		Init: func(t *testcase.T) []string {
			return []string{}
		},
	}

	var makeElem = func(tb testing.TB) string {
		t := tb.(*testcase.T)
		// we produce unique values because a Set type only accept new unique value
		v := random.Unique(func() string { return t.Random.String() }, vs.Get(t)...)
		testcase.Append(t, vs, v)
		return v
	}

	s.Test("#FromSlice", func(t *testcase.T) {
		values := random.Slice(t.Random.IntBetween(0, 10), t.Random.String, random.UniqueValues)
		set := dsset.Set[string]{}.FromSlice(values)
		assert.Equal(t, set.Len(), len(values))
		for _, v := range values {
			assert.True(t, set.Contains(v), "Set should contain the value added from the slice")
		}
	})

	s.Test("#Contains on nil Set", func(t *testcase.T) {
		var set dsset.Set[string]
		assert.False(t, set.Contains(t.Random.String()))
	})

	lc := dscontract.ListConfig[string]{
		MakeElem: makeElem,
	}

	s.Context("implements List", dscontract.List(func(tb testing.TB) ds.List[string] {
		return &dsset.Set[string]{}
	}, lc).Spec)

	s.Context("implements Containable", dscontract.Containable(func(t testing.TB) *dsset.Set[string] {
		return &dsset.Set[string]{}
	}, lc).Spec)

	s.Context("implements Appendable", dscontract.Appendable(func(tb testing.TB) *dsset.Set[string] {
		return &dsset.Set[string]{}
	}).Spec)
}

func ExampleOrderedSet() {
	var set dsset.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.ToSlice()       // []string{"foo", "bar", "baz"}
	set.Len()           // 3
	set.Contains("foo") // true
}

func ExampleOrderedSet_Append() {
	var set dsset.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.ToSlice() // []string{"foo", "bar", "baz"}
	set.Len()     // 3
}

func ExampleOrderedSet_ToSlice() {
	var set dsset.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.ToSlice() // []string{"foo", "bar", "baz"}
	set.Len()     // 3
}

func ExampleOrderedSet_FromSlice() {
	var vs = []string{"foo", "bar", "baz", "foo"}
	var set = dsset.OrderedSet[string]{}.FromSlice(vs)
	set.ToSlice() // []string{"foo", "bar", "baz"}
	set.Len()     // 3
}

func ExampleOrderedSet_Values() {
	var set dsset.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")

	for v := range set.Values() {
		_ = v // "foo" -> "bar" -> "baz"
	}
}

func ExampleOrderedSet_Contains() {
	var set dsset.OrderedSet[string]
	set.Append("foo", "bar", "baz", "foo")
	set.Contains("foo") // true
	set.Contains("bar") // true
	set.Contains("oof") // false
}

func TestOrderedSet(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("Add and Contains", func(t *testing.T) {
		var (
			set      dsset.OrderedSet[int]
			value    = rnd.Int()
			othValue = random.Unique(rnd.Int, value)
		)

		// Initially, the set should not contain the random value
		assert.False(t, set.Contains(value))

		// After adding the value, Contains should return true
		set.Append(value)
		assert.True(t, set.Contains(value))
		assert.False(t, set.Contains(othValue))
	})

	t.Run("FromSlice", func(t *testing.T) {
		values := []int{rnd.Int(), rnd.Int()}
		set := dsset.OrderedSet[int]{}.FromSlice(values)

		for _, v := range values {
			assert.True(t, set.Contains(v), "Set should contain the value added from the slice")
		}
	})

	t.Run("Slice uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := dsset.OrderedSet[int]{}.FromSlice(exp)
		got := set.ToSlice()

		// Create a temporary map to check for duplicates in the slice
		tempMap := make(map[int]struct{})
		for _, item := range got {
			if _, exists := tempMap[item]; exists {
				t.Errorf("Duplicate found in the slice returned by Slice, which should not happen")
			}
			tempMap[item] = struct{}{}
		}
		// Ensure all original unique values are present
		for _, v := range exp {
			_, ok := tempMap[v]

			assert.True(t, ok, assert.MessageF("%v was missing", v),
				"\nAll unique values from the initial slice should be present in the slice returned by Slice.")
		}
	})

	t.Run("FromSlice uniqueness", func(t *testing.T) {
		exp := []int{1, 2, 2, 3} // Intentional duplicate to test uniqueness
		set := dsset.OrderedSet[int]{}.FromSlice(exp)
		got := set.ToSlice()

		// Create a temporary map to check for duplicates in the slice
		tempMap := make(map[int]struct{})
		for _, item := range got {
			if _, exists := tempMap[item]; exists {
				t.Errorf("Duplicate found in the slice returned by Slice, which should not happen")
			}
			tempMap[item] = struct{}{}
		}
		// Ensure all original unique values are present
		for _, v := range exp {
			_, ok := tempMap[v]

			assert.True(t, ok, assert.MessageF("%v was missing", v),
				"\nAll unique values from the initial slice should be present in the slice returned by Slice.")
		}
	})

	t.Run("Slice is ordered by default", func(t *testing.T) {
		exp := []int{1, 5, 2, 7, 3, 9} // Intentional duplicate to test uniqueness
		set := dsset.OrderedSet[int]{}.FromSlice(exp)
		got := set.ToSlice()

		assert.Equal(t, exp, got, "values were expected, and in the same order")
	})

	vs := testcase.Var[[]string]{
		ID: "already used values",
		Init: func(t *testcase.T) []string {
			return []string{}
		},
	}

	var makeElem = func(tb testing.TB) string {
		t := tb.(*testcase.T)
		// we produce unique values because a Set type only accept new unique value
		v := random.Unique(func() string { return t.Random.String() }, vs.Get(t)...)
		testcase.Append(t, vs, v)
		return v
	}

	lc := dscontract.ListConfig[string]{
		MakeElem: makeElem,
	}

	sc := dscontract.SequenceConfig[string]{
		MakeElem: makeElem,
	}

	t.Run("implements List", dscontract.List(func(tb testing.TB) ds.List[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Test)

	t.Run("implements ordered List", dscontract.OrderedList(func(tb testing.TB) ds.List[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Test)

	t.Run("implements sequence", dscontract.Sequence[string](func(tb testing.TB) ds.Sequence[string] {
		return &dsset.OrderedSet[string]{}
	}, sc).Test)

	t.Run("implements Containable", dscontract.Containable(func(t testing.TB) *dsset.OrderedSet[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Test)

	t.Run("implements Appendable", dscontract.Appendable(func(tb testing.TB) *dsset.OrderedSet[string] {
		return &dsset.OrderedSet[string]{}
	}).Test)
}
