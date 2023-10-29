package merge_test

import (
	"go.llib.dev/frameless/pkg/merge"
	"go.llib.dev/testcase/assert"
	"testing"
)

func ExampleSlice() {
	var (
		a       = []int{1, 2, 3}
		b       = []int{7, 8, 9}
		c       = []int{4, 5, 6}
		d []int = nil
	)
	got := merge.Slice(a, b, c, d)
	_ = got // []int{1, 2, 3, 7, 8, 9, 4, 5, 6}
}

func TestSlice(t *testing.T) {
	var (
		a       = []int{1, 2, 3}
		b       = []int{7, 8, 9}
		c       = []int{4, 5, 6}
		d []int = nil
	)
	got := merge.Slice(a, b, c, d)
	assert.Equal(t, got, []int{1, 2, 3, 7, 8, 9, 4, 5, 6})
}

func ExampleMap() {
	var (
		a = map[string]int{"a": 1, "b": 2, "c": 3}
		b = map[string]int{"g": 7, "h": 8, "i": 9}
		c = map[string]int{"d": 4, "e": 5, "f": 6}
		d = map[string]int{"a": 42}
	)
	got := merge.Map(a, b, c, d)
	_ = got
	//
	//	map[string]int{
	//		"a": 42, "b": 2, "c": 3,
	//		"g": 7, "h": 8, "i": 9,
	//		"d": 4, "e": 5, "f": 6,
	//	}
}

func TestMap(t *testing.T) {
	var (
		a = map[string]int{"a": 1, "b": 2, "c": 3}
		b = map[string]int{"g": 7, "h": 8, "i": 9}
		c = map[string]int{"d": 4, "e": 5, "f": 6}
		d = map[string]int{"a": 42}
	)
	got := merge.Map(a, b, c, d)
	assert.Equal(t, got, map[string]int{
		"b": 2, "c": 3,
		"g": 7, "h": 8, "i": 9,
		"d": 4, "e": 5, "f": 6,
		"a": 42,
	})
}
