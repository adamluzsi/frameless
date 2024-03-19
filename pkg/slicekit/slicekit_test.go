package slicekit_test

import (
	"fmt"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/assert"
	"strconv"
	"strings"
	"testing"
)

func ExampleMust() {
	var x = []int{1, 2, 3}
	x = slicekit.Must(slicekit.Map[int](x, func(v int) int {
		return v * 2
	}))

	v := slicekit.Must(slicekit.Reduce[int](x, 42, func(output int, current int) int {
		return output + current
	}))

	fmt.Println("result:", v)
}

func TestMust(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got := slicekit.Must(slicekit.Map[int](x, strconv.Atoi))
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		pv := assert.Panic(t, func() {
			slicekit.Must(slicekit.Map[int](x, strconv.Atoi))
		})
		err, ok := pv.(error)
		assert.True(t, ok)
		assert.Error(t, err)
	})
}

func ExampleMap() {
	var x = []string{"a", "b", "c"}
	_ = slicekit.Must(slicekit.Map[string](x, strings.ToUpper)) // []string{"A", "B", "C"}

	var ns = []string{"1", "2", "3"}
	_, err := slicekit.Map[int](ns, strconv.Atoi) // []int{1, 2, 3}
	if err != nil {
		panic(err)
	}
}

func TestMap(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got, err := slicekit.Map[string](x, strings.ToUpper)
		assert.NoError(t, err)
		assert.Equal(t, []string{"A", "B", "C"}, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := slicekit.Map[int](x, strconv.Atoi)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		_, err := slicekit.Map[int](x, strconv.Atoi)
		assert.Error(t, err)
	})
}

func ExampleReduce() {
	var x = []string{"a", "b", "c"}
	got, err := slicekit.Reduce[string](x, "|", func(o string, i string) string {
		return o + i
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(got) // "|abc"
}

func TestReduce(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got, err := slicekit.Reduce[string](x, "|", func(o string, i string) string {
			return o + i
		})
		assert.NoError(t, err)
		assert.Equal(t, "|abc", got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := slicekit.Reduce[int](x, 42, func(o int, i string) (int, error) {
			n, err := strconv.Atoi(i)
			if err != nil {
				return o, err
			}
			return o + n, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 42+1+2+3, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		_, err := slicekit.Reduce[int](x, 0, func(o int, i string) (int, error) {
			n, err := strconv.Atoi(i)
			if err != nil {
				return o, err
			}
			return o + n, nil
		})
		assert.Error(t, err)
	})
}

func ExampleLookup() {
	vs := []int{2, 4, 8, 16}
	slicekit.Lookup(vs, 0)      // -> return 2, true
	slicekit.Lookup(vs, 0-1)    // lookup previous -> return 0, false
	slicekit.Lookup(vs, 0+1)    // lookup next -> return 4, true
	slicekit.Lookup(vs, 0+1000) // lookup 1000th element -> return 0, false
}

func TestLookup_smoke(t *testing.T) {
	vs := []int{2, 4, 8, 16}

	v, ok := slicekit.Lookup(vs, 0)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 2)

	v, ok = slicekit.Lookup(vs, 0-1)
	assert.Equal(t, ok, false)
	assert.Equal(t, v, 0)

	v, ok = slicekit.Lookup(vs, 0+1)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 4)

	v, ok = slicekit.Lookup(vs, 0+1000)
	assert.Equal(t, ok, false)
	assert.Equal(t, v, 0)

	v, ok = slicekit.Lookup(vs, 0+1000)
	assert.Equal(t, ok, false)
	assert.Equal(t, v, 0)

	for i, exp := range vs {
		got, ok := slicekit.Lookup(vs, i)
		assert.Equal(t, ok, true)
		assert.Equal(t, exp, got)
	}
}

func TestMerge(t *testing.T) {
	t.Run("all slice merged into one", func(t *testing.T) {
		var (
			a   = []string{"a", "b", "c"}
			b   = []string{"1", "2", "3"}
			c   = []string{"1", "B", "3"}
			out = slicekit.Merge(a, b, c)
		)
		assert.Equal(t, out, []string{
			"a", "b", "c",
			"1", "2", "3",
			"1", "B", "3",
		})
	})
	t.Run("input slices are not affected by the merging process", func(t *testing.T) {
		var (
			a = []string{"a", "b", "c"}
			b = []string{"1", "2", "3"}
			c = []string{"1", "B", "3"}
			_ = slicekit.Merge(a, b, c)
		)
		assert.Equal(t, a, []string{"a", "b", "c"})
		assert.Equal(t, b, []string{"1", "2", "3"})
		assert.Equal(t, c, []string{"1", "B", "3"})
	})
}
