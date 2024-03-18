package mapkit_test

import (
	"fmt"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/testcase/assert"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func ExampleMust() {
	var x = map[string]int{"A": 1, "B": 2, "C": 3}

	x = mapkit.Must(mapkit.Map[string, int](x,
		func(k string, v int) (string, int) { return k, v * 2 }))

	v := mapkit.Must(mapkit.Reduce[int](x, 42,
		func(output int, k string, v int) int { return output + v }))

	fmt.Println("result:", v)
}

func TestMust(t *testing.T) {
	mapper := func(k, v string) (string, int, error) {
		cv, err := strconv.Atoi(v)
		return k, cv, err
	}
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got := mapkit.Must(mapkit.Map[string, int](x, mapper))
		assert.Equal(t, map[string]int{"a": 1, "b": 2, "c": 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "two", "c": "3"}
		pv := assert.Panic(t, func() {
			_ = mapkit.Must(mapkit.Map[string, int](x, mapper))
		})
		err, ok := pv.(error)
		assert.True(t, ok)
		assert.Error(t, err)
	})
}

func ExampleMap() {
	var x = map[string]string{"a": "x", "b": "y", "c": "z"}

	x = mapkit.Must(mapkit.Map[string, string](x,
		func(k string, v string) (string, string) { return k, strings.ToUpper(v) }))

	fmt.Printf("%#v\n", x) // map[string]string{"a": "X", "b": "Y", "c": "Z"}
}

func ExampleMap_withError() {
	var x = map[string]string{"a": "1", "b": "2", "c": "3"}

	mx, err := mapkit.Map[string, int](x, func(k, v string) (string, int, error) {
		cv, err := strconv.Atoi(v)
		return k, cv, err
	})
	if err != nil {
		panic("handle error")
	}

	fmt.Printf("%#v\n", mx) // map[string]int{"a": 1, "b": 2, "c": 3}
}

func TestMap(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = map[string]string{"a": "x", "b": "y", "c": "z"}
		got, err := mapkit.Map[string, string](x,
			func(k string, v string) (string, string) {
				return k, strings.ToUpper(v)
			})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"a": "X", "b": "Y", "c": "Z"}, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got, err := mapkit.Map[string, int](x, func(k, v string) (string, int, error) {
			cv, err := strconv.Atoi(v)
			return k, cv, err
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]int{"a": 1, "b": 2, "c": 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "x", "c": "3"}
		_, err := mapkit.Map[string, int](x, func(k, v string) (string, int, error) {
			cv, err := strconv.Atoi(v)
			return k, cv, err
		})
		assert.Error(t, err)
	})
}

func ExampleReduce() {
	var x = map[string]int{"a": 1, "b": 2, "c": 3}
	got, err := mapkit.Reduce[int](x, 0, func(o int, k string, v int) int {
		return o + v
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(got) // "|abc"
}

func TestReduce(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = map[string]int{"a": 1, "b": 2, "c": 3}
		got, err := mapkit.Reduce[int](x, 42, func(o int, k string, v int) int {
			return o + v
		})
		assert.NoError(t, err)
		assert.Equal(t, 42+1+2+3, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = map[string]string{"a": "1", "b": "2", "c": "3"}
		got, err := mapkit.Reduce[int](x, 42, func(o int, k, v string) (int, error) {
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
		_, err := mapkit.Reduce[int](x, 42, func(o int, k, v string) (int, error) {
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
	assert.ContainExactly(t, []string{"a", "b", "c"}, mapkit.Keys(map[string]string{"a": "1", "b": "2", "c": "3"}))
	assert.ContainExactly(t, []int{42, 32, 88}, mapkit.Keys(map[int]string{42: "A", 32: "B", 88: "C"}))
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
