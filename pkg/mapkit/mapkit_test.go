package mapkit_test

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/testcase/assert"
)

func ExampleMust() {
	var x = map[string]int{"A": 1, "B": 2, "C": 3}

	x = must.Must(mapkit.MapErr[string, int](x,
		func(k string, v int) (string, int, error) { return k, v * 2, nil }))

	v := must.Must(mapkit.ReduceErr[int](x, 42,
		func(output int, k string, v int) (int, error) { return output + v, nil }))

	fmt.Println("result:", v)
}

func TestMust(t *testing.T) {
	mapper := func(k, v string) (string, int, error) {
		cv, err := strconv.Atoi(v)
		return k, cv, err
	}
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got := must.Must(mapkit.MapErr[string, int](x, mapper))
		assert.Equal(t, map[string]int{"a": 1, "b": 2, "c": 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "two", "c": "3"}
		pv := assert.Panic(t, func() {
			_ = must.Must(mapkit.MapErr[string, int](x, mapper))
		})
		err, ok := pv.(error)
		assert.True(t, ok)
		assert.Error(t, err)
	})
}

func ExampleMap() {
	var x = map[string]string{"a": "x", "b": "y", "c": "z"}

	x = mapkit.Map(x, func(k string, v string) (string, string) { return k, strings.ToUpper(v) })

	fmt.Printf("%#v\n", x) // map[string]string{"a": "X", "b": "Y", "c": "Z"}
}

func TestMap(t *testing.T) {
	t.Run("zero but not nil kv", func(t *testing.T) {
		var x = map[string]string{}
		got := mapkit.Map(x, func(k string, v string) (string, string) {
			return k, strings.ToUpper(v)
		})
		assert.NotNil(t, got)
		assert.Equal(t, got, map[string]string{})
	})
	t.Run("many kv", func(t *testing.T) {
		var x = map[string]string{"a": "x", "b": "y", "c": "z"}
		got := mapkit.Map(x, func(k string, v string) (string, string) {
			return k, strings.ToUpper(v)
		})
		assert.Equal(t, map[string]string{"a": "X", "b": "Y", "c": "Z"}, got)
	})
}

func ExampleMapErr() {
	var x = map[string]string{"a": "x", "b": "y", "c": "z"}

	x = must.Must(mapkit.MapErr[string, string](x,
		func(k string, v string) (string, string, error) { return k, strings.ToUpper(v), nil }))

	fmt.Printf("%#v\n", x) // map[string]string{"a": "X", "b": "Y", "c": "Z"}
}

func ExampleMapErr_withError() {
	var x = map[string]string{"a": "1", "b": "2", "c": "3"}

	mx, err := mapkit.MapErr[string, int](x, func(k, v string) (string, int, error) {
		cv, err := strconv.Atoi(v)
		return k, cv, err
	})
	if err != nil {
		panic("handle error")
	}

	fmt.Printf("%#v\n", mx) // map[string]int{"a": 1, "b": 2, "c": 3}
}

func TestMapErr(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = map[string]string{"a": "x", "b": "y", "c": "z"}
		got, err := mapkit.MapErr[string, string](x,
			func(k string, v string) (string, string, error) {
				return k, strings.ToUpper(v), nil
			})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"a": "X", "b": "Y", "c": "Z"}, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got, err := mapkit.MapErr[string, int](x, func(k, v string) (string, int, error) {
			cv, err := strconv.Atoi(v)
			return k, cv, err
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]int{"a": 1, "b": 2, "c": 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "x", "c": "3"}
		_, err := mapkit.MapErr[string, int](x, func(k, v string) (string, int, error) {
			cv, err := strconv.Atoi(v)
			return k, cv, err
		})
		assert.Error(t, err)
	})
}

func ExampleReduce() {
	var x = map[string]int{"a": 1, "b": 2, "c": 3}
	got := mapkit.Reduce[int](x, 0, func(o int, k string, v int) int {
		return o + v
	})
	fmt.Println(got) // "|abc"
}

func TestReduce(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var x = map[string]int{}
		got := mapkit.Reduce[int](x, 42, func(o int, k string, v int) int { return o + v })
		assert.Equal(t, 42, got)
	})
	t.Run("many kvs", func(t *testing.T) {
		var x = map[string]int{"a": 1, "b": 2, "c": 3}
		got := mapkit.Reduce[int](x, 42, func(o int, k string, v int) int {
			return o + v
		})
		assert.Equal(t, 42+1+2+3, got)
	})
}

func ExampleReduceErr() {
	var x = map[string]int{"a": 1, "b": 2, "c": 3}
	got, err := mapkit.ReduceErr[int](x, 0, func(o int, k string, v int) (int, error) {
		return o + v, nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(got) // "|abc"
}

func TestReduceErr(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = map[string]int{"a": 1, "b": 2, "c": 3}
		got, err := mapkit.ReduceErr[int](x, 42, func(o int, k string, v int) (int, error) {
			return o + v, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 42+1+2+3, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got, err := mapkit.ReduceErr[int](x, 42, func(o int, k, v string) (int, error) {
			n, err := strconv.Atoi(v)
			if err != nil {
				return o, err
			}
			return o + n, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 42+1+2+3, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "X", "c": "3"}
		_, err := mapkit.ReduceErr[int](x, 42, func(o int, k, v string) (int, error) {
			n, err := strconv.Atoi(v)
			if err != nil {
				return o, err
			}
			return o + n, nil
		})
		assert.Error(t, err)
	})
}

func ExampleKeys() {
	_ = mapkit.Keys(map[string]string{"a": "1", "b": "2", "c": "3"})
	// -> []string{"a", "b", "c"}
}

func TestKeys(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		assert.ContainExactly(t, []string{"a", "b", "c"}, mapkit.Keys(map[string]string{"a": "1", "b": "2", "c": "3"}))
		assert.ContainExactly(t, []int{42, 32, 88}, mapkit.Keys(map[int]string{42: "A", 32: "B", 88: "C"}))
	})
	t.Run("with sort", func(t *testing.T) {
		m := map[int]string{42: "A", 32: "B", 88: "C"}
		ks := mapkit.Keys(m, sort.Ints)
		assert.Equal(t, ks, []int{32, 42, 88})
	})
}

func ExampleMerge() {
	var (
		a = map[string]int{"a": 1, "b": 2, "c": 3}
		b = map[string]int{"g": 7, "h": 8, "i": 9}
		c = map[string]int{"d": 4, "e": 5, "f": 6}
		d = map[string]int{"a": 42}
	)
	got := mapkit.Merge(a, b, c, d)
	_ = got
	//
	//	map[string]int{
	//		"a": 42, "b": 2, "c": 3,
	//		"g": 7, "h": 8, "i": 9,
	//		"d": 4, "e": 5, "f": 6,
	//	}
}

func TestMerge(t *testing.T) {
	t.Run("input maps are not affected", func(t *testing.T) {
		var (
			a   = map[string]int{"a": 1, "b": 2, "c": 3}
			b   = map[string]int{"A": 11, "B": 22, "C": 33}
			c   = map[string]int{"a": 42}
			got = mapkit.Merge(a, b, c)
		)
		assert.Equal(t, a, map[string]int{"a": 1, "b": 2, "c": 3}, "input map was not expected to change")
		assert.Equal(t, b, map[string]int{"A": 11, "B": 22, "C": 33}, "input map was not expected to change")
		assert.Equal(t, c, map[string]int{"a": 42}, "input map was not expected to change")
		assert.Equal(t, got, map[string]int{"a": 42, "b": 2, "c": 3, "A": 11, "B": 22, "C": 33})
	})
	t.Run("output has every element merged in, and priorotised by arg order, where last is the most important", func(t *testing.T) {
		var (
			a = map[string]int{"a": 1, "b": 2, "c": 3}
			b = map[string]int{"g": 7, "h": 8, "i": 9}
			c = map[string]int{"d": 4, "e": 5, "f": 6}
			d = map[string]int{"a": 42}
		)
		got := mapkit.Merge(a, b, c, d)
		assert.Equal(t, got, map[string]int{
			"b": 2, "c": 3,
			"g": 7, "h": 8, "i": 9,
			"d": 4, "e": 5, "f": 6,
			"a": 42,
		})
	})
}

func ExampleClone() {
	var (
		src = map[string]int{"a": 1, "b": 2, "c": 3}
		dst = mapkit.Clone(src)
	)
	_ = dst // dst == map[string]int{"a": 1, "b": 2, "c": 3}
}

func TestClone(t *testing.T) {
	t.Run("clones a value", func(t *testing.T) {
		var (
			src = map[string]int{"a": 1, "b": 2, "c": 3}
			dst = mapkit.Clone(src)
		)
		assert.Equal(t, src, dst)
	})
	t.Run("altering the cloned map doesn't affect the source map", func(t *testing.T) {
		var (
			src = map[string]int{"a": 1, "b": 2, "c": 3}
			dst = mapkit.Clone(src)
		)
		dst["n"] = 42
		assert.Equal(t, src, map[string]int{"a": 1, "b": 2, "c": 3})
		assert.Equal(t, dst, map[string]int{"a": 1, "b": 2, "c": 3, "n": 42})
	})
	t.Run("map suptype can be cloned, even though while losing the concrete subtype", func(t *testing.T) {
		type MyMap map[string]int
		var (
			src = MyMap{"a": 1, "b": 2, "c": 3}
			dst = mapkit.Clone(src)
		)
		assert.Equal[MyMap](t, src, dst)
		assert.Equal(t, reflect.TypeOf(src), reflect.TypeOf(dst))
	})
}

func ExampleFilter() {
	var (
		src = map[int]string{1: "a", 2: "b", 3: "c"}
		dst = mapkit.Filter[int, string](src, func(k int, v string) bool {
			return k != 2
		})
	)
	_ = dst // map[int]string{1: "a", 3: "c"}, nil
}

func TestFilter(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var (
			src = map[int]string{}
			dst = mapkit.Filter(src, func(k int, v string) bool {
				return k != 2
			})
		)
		assert.NotNil(t, dst)
		assert.Equal(t, dst, map[int]string{})
	})
	t.Run("many kvs", func(t *testing.T) {
		var (
			src = map[int]string{1: "a", 2: "b", 3: "c"}
			dst = mapkit.Filter(src, func(k int, v string) bool {
				return k != 2
			})
		)
		assert.Equal(t, src, map[int]string{1: "a", 2: "b", 3: "c"})
		assert.Equal(t, dst, map[int]string{1: "a", 3: "c"})
	})
}

func ExampleFilterErr() {
	var (
		src      = map[int]string{1: "a", 2: "b", 3: "c"}
		dst, err = mapkit.FilterErr[int, string](src, func(k int, v string) (bool, error) {
			return k != 2, nil
		})
	)
	_, _ = dst, err // map[int]string{1: "a", 3: "c"}, nil
}

func TestFilterErr(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			src      = map[int]string{1: "a", 2: "b", 3: "c"}
			dst, err = mapkit.FilterErr[int, string](src, func(k int, v string) (bool, error) {
				return k != 2, nil
			})
		)
		assert.NoError(t, err)
		assert.Equal(t, src, map[int]string{1: "a", 2: "b", 3: "c"})
		assert.Equal(t, dst, map[int]string{1: "a", 3: "c"})
	})
	t.Run("happy (no-error)", func(t *testing.T) {
		var (
			src = map[int]string{1: "a", 2: "b", 3: "c"}
			dst = must.Must(mapkit.FilterErr[int, string](src, func(k int, v string) (bool, error) {
				return k != 2, nil
			}))
		)
		assert.Equal(t, src, map[int]string{1: "a", 2: "b", 3: "c"})
		assert.Equal(t, dst, map[int]string{1: "a", 3: "c"})
	})
	t.Run("error is propagated back", func(t *testing.T) {
		expErr := fmt.Errorf("boom")
		got, err := mapkit.FilterErr[int, string](map[int]string{1: "a"}, func(k int, v string) (bool, error) {
			return false, expErr
		})
		assert.ErrorIs(t, err, expErr)
		assert.Empty(t, got)
	})
}

func ExampleValues() {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	vs := mapkit.Values(m)
	_ = vs // []string{"a", "b", "c"} // order not guaranteed
}

func ExampleValues_sorted() {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	mapkit.Values(m, sort.Strings)
}

func TestValues(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		m := map[int]string{1: "a", 2: "b", 3: "c"}
		vs := mapkit.Values(m)
		assert.ContainExactly(t, vs, []string{"a", "b", "c"})
	})
	t.Run("with sort", func(t *testing.T) {
		m := map[int]string{1: "a", 2: "b", 3: "c"}
		vs := mapkit.Values(m, sort.Strings)
		assert.Equal(t, vs, []string{"a", "b", "c"})
	})
	t.Run("sort nil", func(t *testing.T) {
		var m map[int]string
		vs := mapkit.Values(m, sort.Strings)
		assert.Empty(t, vs)
	})
	t.Run("with sort func", func(t *testing.T) {
		var m map[int]string
		vs := mapkit.Values(m, func(s []string) {
			slices.SortFunc[[]string](s, func(a, b string) int {
				switch {
				case a == b:
					return 0
				case a < b:
					return -1
				case a > b:
					return 1
				default:
					panic("not implemented")
				}
			})
		})
		assert.Empty(t, vs)
	})
}

func TestToSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, mapkit.ToSlice[string, int](nil))
	})

	t.Run("empty", func(t *testing.T) {
		vs := mapkit.ToSlice(map[string]int{})
		assert.NotNil(t, vs)
		assert.Empty(t, vs)
	})

	t.Run("smoke", func(t *testing.T) {
		input := map[string]int{"foo": 1, "bar": 2, "baz": 3}
		output := mapkit.ToSlice(input)

		assert.ContainExactly(t, output, []mapkit.Entry[string, int]{
			{Key: "foo", Value: 1},
			{Key: "bar", Value: 2},
			{Key: "baz", Value: 3},
		})
	})
}

func TestLookup(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var vs map[int]string
		v, ok := mapkit.Lookup(vs, 42)
		assert.False(t, ok)
		assert.Empty(t, v)
	})
	t.Run("missing", func(t *testing.T) {
		var vs = map[int]string{}
		v, ok := mapkit.Lookup(vs, 42)
		assert.False(t, ok)
		assert.Empty(t, v)
	})
	t.Run("contain", func(t *testing.T) {
		var vs = map[int]string{42: "foo"}
		v, ok := mapkit.Lookup(vs, 42)
		assert.True(t, ok)
		assert.Equal(t, v, "foo")
	})
	t.Run("maptype", func(t *testing.T) {
		type M map[int]string
		var vs = M{42: "foo"}
		v, ok := mapkit.Lookup(vs, 42)
		assert.True(t, ok)
		assert.Equal(t, v, "foo")
	})
}
