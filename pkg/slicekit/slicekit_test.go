package slicekit_test

import (
	"fmt"
	"iter"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type CustomSliceType []int

func Example_customType() {
	var vs CustomSliceType
	var o CustomSliceType
	_ = o
	o = slicekit.Clone(vs)
	slicekit.Insert(&vs, 0, 1, 2, 3)
	slicekit.Set(vs, 0, 42)
	slicekit.Delete(&vs, -1) // pop
	o = slicekit.Filter(vs, func(v int) bool { return true })
	o, _ = slicekit.FilterErr(vs, func(v int) (bool, error) { return true, nil })
	slicekit.GroupBy(vs, func(n int) int { return n })
	o = slicekit.Merge(vs, vs)
	_, _ = slicekit.Pop(&vs)
	_, _ = slicekit.PopAt(&vs, -1)
}

func ExampleMust() {
	var x = []int{1, 2, 3}
	x = must.Must(slicekit.MapErr[int](x, func(v int) (int, error) {
		return v * 2, nil
	}))

	v := must.Must(slicekit.ReduceErr[int](x, 42, func(output int, current int) (int, error) {
		return output + current, nil
	}))

	fmt.Println("result:", v)
}

func TestMust(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got := must.Must(slicekit.MapErr[int](x, strconv.Atoi))
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		pv := assert.Panic(t, func() {
			must.Must(slicekit.MapErr[int](x, strconv.Atoi))
		})
		err, ok := pv.(error)
		assert.True(t, ok)
		assert.Error(t, err)
	})
}

func ExampleMap() {
	var x = []string{"a", "b", "c"}
	_ = slicekit.Map(x, strings.ToUpper) // []string{"A", "B", "C"}

	var ns = []string{"1", "2", "3"}
	_, err := slicekit.MapErr[int](ns, strconv.Atoi) // []int{1, 2, 3}
	if err != nil {
		panic(err)
	}
}

func ExampleMapErr() {
	var x = []string{"a", "b", "c"}
	_ = must.Must(slicekit.MapErr[string](x, func(s string) (string, error) {
		return strings.ToUpper(s), nil
	})) // []string{"A", "B", "C"}

	var ns = []string{"1", "2", "3"}
	_, err := slicekit.MapErr[int](ns, strconv.Atoi) // []int{1, 2, 3}
	if err != nil {
		panic(err)
	}
}

func TestMap(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got := slicekit.Map(x, strings.ToUpper)
		assert.Equal(t, []string{"A", "B", "C"}, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []int{1, 2, 3}
		got := slicekit.Map(x, strconv.Itoa)
		assert.Equal(t, []string{"1", "2", "3"}, got)
	})
}

func TestMapErr(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := slicekit.MapErr[int](x, strconv.Atoi)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		_, err := slicekit.MapErr[int](x, strconv.Atoi)
		assert.Error(t, err)
	})
}

func ExampleReduce() {
	var x = []int{1, 2, 3}
	got := slicekit.Reduce[string](x, "|", func(s string, i int) string {
		return s + strconv.Itoa(i)
	})
	fmt.Println(got) // "|123"
}

func ExampleReduceErr() {
	var x = []string{"a", "b", "c"}
	got, err := slicekit.ReduceErr[string](x, "|", func(o string, i string) (string, error) {
		return o + i, nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(got) // "|abc"
}

func TestReduce(t *testing.T) {
	t.Run("zero elements", func(t *testing.T) {
		var x = []string{}
		got := slicekit.Reduce(x, "|", func(o string, i string) string {
			return o + i
		})
		assert.Equal(t, "|", got)
	})
	t.Run("one element", func(t *testing.T) {
		var x = []string{"a"}
		got := slicekit.Reduce(x, "|", func(o string, i string) string {
			return o + i
		})
		assert.Equal(t, "|a", got)
	})
	t.Run("many elements", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got := slicekit.Reduce(x, "|", func(o string, i string) string {
			return o + i
		})
		assert.Equal(t, "|abc", got)
	})
}

func TestReduceErr(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got, err := slicekit.ReduceErr[string](x, "|", func(o string, i string) (string, error) {
			return o + i, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, "|abc", got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := slicekit.ReduceErr[int](x, 42, func(o int, i string) (int, error) {
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
		_, err := slicekit.ReduceErr[int](x, 0, func(o int, i string) (int, error) {
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
	slicekit.Lookup(vs, 0)  // 2
	slicekit.Lookup(vs, 1)  // 4
	slicekit.Lookup(vs, 2)  // 8
	slicekit.Lookup(vs, -1) // 16 (last)
	slicekit.Lookup(vs, -2) // 8  (second last)
}

func TestLookup_smoke(t *testing.T) {
	vs := []int{2, 4, 8, 16}

	v, ok := slicekit.Lookup(vs, 0)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 2)

	v, ok = slicekit.Lookup(vs, -1)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 16)

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

func TestLookup_negativeIndex(t *testing.T) {
	vs := []int{2, 4, 8, 16, 32}

	v, ok := slicekit.Lookup(vs, -1)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 32)

	v, ok = slicekit.Lookup(vs, -2)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 16)

	v, ok = slicekit.Lookup(vs, -3)
	assert.Equal(t, ok, true)
	assert.Equal(t, v, 8)

	v, ok = slicekit.Lookup(vs, (len(vs)+1)*-1)
	assert.Equal(t, ok, false)
	assert.Empty(t, v)
}

func ExampleGet() {
	vs := []int{2, 4, 8, 16}
	slicekit.Get(vs, 0) // 2
	slicekit.Get(vs, 1) // 4
	slicekit.Get(vs, 2) // 8
	slicekit.Get(vs, 3) // 16

	slicekit.Get(vs, -1) // 16 (last)
	slicekit.Get(vs, -2) // 16 (second last)

	slicekit.Get(vs, 0+1000) // 0 (zero value for not found)
}

func TestGet(t *testing.T) {
	t.Run("pos", func(t *testing.T) {
		vs := []int{2, 4, 8, 16}

		v := slicekit.Get(vs, 0)
		assert.Equal(t, v, 2)

		v = slicekit.Get(vs, -1)
		assert.Equal(t, v, 16)

		v = slicekit.Get(vs, 0+1)
		assert.Equal(t, v, 4)

		v = slicekit.Get(vs, 0+1000)
		assert.Empty(t, v)

		v = slicekit.Get(vs, 0+1000)
		assert.Empty(t, v)

		for i, exp := range vs {
			got := slicekit.Get(vs, i)
			assert.Equal(t, exp, got)
		}
	})

	t.Run("neg", func(t *testing.T) {
		vs := []int{2, 4, 8, 16, 32}

		v := slicekit.Get(vs, -1)
		assert.Equal(t, v, 32)

		v = slicekit.Get(vs, -2)
		assert.Equal(t, v, 16)

		v = slicekit.Get(vs, -3)
		assert.Equal(t, v, 8)

		v = slicekit.Get(vs, (len(vs)+1)*-1)
		assert.Empty(t, v)
	})
}

func ExampleMerge() {
	var (
		a   = []string{"a", "b", "c"}
		b   = []string{"1", "2", "3"}
		c   = []string{"1", "B", "3"}
		out = slicekit.Merge(a, b, c)
	)
	_ = out // []string{"a", "b", "c", "1", "2", "3", "1", "B", "3"}
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

func ExampleClone() {
	var (
		src = []string{"a", "b", "c"}
		dst = slicekit.Clone(src)
	)
	_, _ = src, dst
}

func TestClone(t *testing.T) {
	t.Run("clone will creates an identical copy of the source slice", func(t *testing.T) {
		var (
			src = []string{"a", "b", "c"}
			dst = slicekit.Clone(src)
		)
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "b", "c"})
	})
	t.Run("original slice is not modified when its clone is altered", func(t *testing.T) {
		var (
			src = []string{"a", "b", "c"}
			dst = slicekit.Clone(src)
		)
		dst[1] = "42"
		dst = append(dst, "foo")
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "42", "c", "foo"})
	})
	t.Run("nil slice clones into a nil slice", func(t *testing.T) {
		assert.Equal(t, slicekit.Clone[[]int](nil), nil)
	})
}

func ExampleFilter() {
	var (
		src = []string{"a", "b", "c"}
		dst = slicekit.Filter(src, func(s string) bool {
			return s != "c"
		})
	)
	_ = dst // []string{"a", "b"}, nil
}

func TestFilter(t *testing.T) {
	t.Run("", func(t *testing.T) {
		var (
			src = []string{"a", "b", "c"}
			dst = slicekit.Filter(src, func(s string) bool {
				return s != "c"
			})
		)
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "b"})
	})
	t.Run("", func(t *testing.T) {
		var (
			src = []string{"a", "b", "c"}
			dst = slicekit.Filter(src, func(s string) bool {
				return s != "b"
			})
		)
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "c"})
	})
}

func ExampleFilterErr() {
	var (
		src      = []string{"a", "b", "c"}
		dst, err = slicekit.FilterErr(src, func(s string) (bool, error) {
			return s != "c", nil
		})
	)
	_, _ = dst, err // []string{"a", "b"}, nil
}

func TestFilterErr(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			src      = []string{"a", "b", "c"}
			dst, err = slicekit.FilterErr(src, func(s string) (bool, error) {
				return s != "c", nil
			})
		)
		assert.NoError(t, err)
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "b"})
	})
	t.Run("happy (no-error)", func(t *testing.T) {
		var (
			src = []string{"a", "b", "c"}
			dst = must.Must(slicekit.FilterErr(src, func(s string) (bool, error) {
				return s != "b", nil
			}))
		)
		assert.Equal(t, src, []string{"a", "b", "c"})
		assert.Equal(t, dst, []string{"a", "c"})
	})
	t.Run("error is propagated back", func(t *testing.T) {
		expErr := fmt.Errorf("boom")
		got, err := slicekit.FilterErr([]string{"a", "b", "c"}, func(s string) (bool, error) {
			return false, expErr
		})
		assert.ErrorIs(t, err, expErr)
		assert.Empty(t, got)
	})
}

func ExampleContains() {
	_ = slicekit.Contains([]string{"foo", "bar", "baz"}, "bar") // true
	_ = slicekit.Contains([]int{7, 42, 128}, 128)               // true
	_ = slicekit.Contains([]int{7, 42, 128}, 32)                // false
}

func ExampleBatch() {
	vs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	batches := slicekit.Batch(vs, 5)
	_ = batches
	// 0 -> []int{1, 2, 3, 4, 5}
	// 1 -> []int{6, 7, 8, 9, 10}
	// 2 -> []int{11, 12, 13, 14, 15}
}

func TestBatch(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		vs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		batches := slicekit.Batch(vs, 5)
		assert.NotEmpty(t, batches)
		assert.True(t, len(batches) == 4)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, batches[0])
		assert.Equal(t, []int{6, 7, 8, 9, 10}, batches[1])
		assert.Equal(t, []int{11, 12, 13, 14, 15}, batches[2])
		assert.Equal(t, []int{16}, batches[3])
	})

	t.Run("exact batch size", func(t *testing.T) {
		vs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		batches := slicekit.Batch(vs, 5)
		assert.NotEmpty(t, batches)
		assert.True(t, len(batches) == 2)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, batches[0])
		assert.Equal(t, []int{6, 7, 8, 9, 10}, batches[1])
	})

	t.Run("non-exact batch size", func(t *testing.T) {
		vs := []int{1, 2, 3, 4, 5, 6, 7}
		batches := slicekit.Batch(vs, 3)
		assert.NotEmpty(t, batches)
		assert.True(t, len(batches) == 3)
		assert.Equal(t, []int{1, 2, 3}, batches[0])
		assert.Equal(t, []int{4, 5, 6}, batches[1])
		assert.Equal(t, []int{7}, batches[2])
	})

	t.Run("empty slice", func(t *testing.T) {
		vs := []int{}
		batches := slicekit.Batch(vs, 3)
		assert.Empty(t, batches)
	})

	t.Run("batch size larger than slice", func(t *testing.T) {
		vs := []int{1, 2, 3}
		batches := slicekit.Batch(vs, 5)
		assert.NotEmpty(t, batches)
		assert.True(t, len(batches) == 1)
		assert.Equal(t, []int{1, 2, 3}, batches[0])
	})

	t.Run("non-exact batch size", func(t *testing.T) {
		vs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
		batches := slicekit.Batch(vs, 4)
		assert.NotEmpty(t, batches)
		assert.True(t, len(batches) == 3)
		assert.Equal(t, []int{1, 2, 3, 4}, batches[0])
		assert.Equal(t, []int{5, 6, 7, 8}, batches[1])
		assert.Equal(t, []int{9}, batches[2])
	})
}

func ExampleUnique() {
	slicekit.Unique([]int{1, 2, 2, 3, 3, 3})
	// -> []int{1, 2, 3}
}

func TestUnique(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		assert.Empty(t, slicekit.Unique([]int{}))
	})

	t.Run("single element", func(t *testing.T) {
		assert.Equal(t, slicekit.Unique([]int{1}), []int{1})
	})

	t.Run("no duplicates", func(t *testing.T) {
		assert.Equal(t, slicekit.Unique([]int{1, 2, 3}), []int{1, 2, 3})
	})

	t.Run("duplicates", func(t *testing.T) {
		assert.Equal(t, slicekit.Unique([]int{1, 2, 2, 3, 3, 3}), []int{1, 2, 3})
	})

	t.Run("string slice", func(t *testing.T) {
		assert.Equal(t, slicekit.Unique([]string{"a", "b", "c"}), []string{"a", "b", "c"})
	})

	t.Run("order based on first occurence", func(t *testing.T) {
		assert.Equal(t, slicekit.Unique([]int{3, 1, 2, 2, 3, 3, 3}), []int{3, 1, 2})
	})

	t.Run("struct slice", func(t *testing.T) {
		type person struct{ name string }
		p1 := person{name: "John"}
		p2 := person{name: "Jane"}
		assert.Equal(t, slicekit.Unique([]person{p1, p2, p1}), []person{p1, p2})
	})

	t.Run("nil input", func(t *testing.T) {
		var nilSlice []int
		assert.Empty(t, slicekit.Unique(nilSlice))
	})
}

func ExampleUniqueBy() {
	type T struct {
		ID  int
		Val string
	}
	vs := []T{
		{ID: 1, Val: "foo1"},
		{ID: 2, Val: "bar1"},
		{ID: 2, Val: "bar2"},
		{ID: 3, Val: "baz1"},
		{ID: 3, Val: "baz2"},
		{ID: 3, Val: "baz3"},
	}
	slicekit.UniqueBy(vs, func(v T) int { return v.ID })
	// []T{
	//   {ID: 1, Val: "foo1"},
	//   {ID: 2, Val: "bar1"},
	//   {ID: 3, Val: "baz1"},
	// }
}

func uniqueBySelf[T comparable](v T) T {
	return v
}

func TestUniqueBy(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		assert.Empty(t, slicekit.UniqueBy([]int{}, uniqueBySelf[int]))
	})

	t.Run("single element", func(t *testing.T) {
		assert.Equal(t, slicekit.UniqueBy([]int{1}, uniqueBySelf[int]), []int{1})
	})

	t.Run("no duplicates", func(t *testing.T) {
		assert.Equal(t, slicekit.UniqueBy([]int{1, 2, 3}, uniqueBySelf[int]), []int{1, 2, 3})
	})

	t.Run("duplicates", func(t *testing.T) {
		assert.Equal(t, slicekit.UniqueBy([]int{1, 2, 2, 3, 3, 3}, uniqueBySelf[int]), []int{1, 2, 3})
	})

	t.Run("string slice", func(t *testing.T) {
		assert.Equal(t, slicekit.UniqueBy([]string{"a", "b", "c"}, uniqueBySelf[string]), []string{"a", "b", "c"})
	})

	t.Run("order based on first occurence", func(t *testing.T) {
		assert.Equal(t, slicekit.UniqueBy([]int{3, 1, 2, 2, 3, 3, 3}, uniqueBySelf[int]), []int{3, 1, 2})
	})

	t.Run("struct slice", func(t *testing.T) {
		type person struct {
			ID   string
			Name string
		}
		p1a := person{ID: "1", Name: "John Jane"}
		p1b := person{ID: "1", Name: "Jane John"}
		p2 := person{ID: "2", Name: "Mr Bob"}
		assert.Equal(t, slicekit.UniqueBy([]person{p1a, p1b, p2}, func(p person) string { return p.ID }), []person{p1a, p2})
	})

	t.Run("nil input", func(t *testing.T) {
		var nilSlice []int
		assert.Empty(t, slicekit.UniqueBy(nilSlice, uniqueBySelf[int]))
	})
}

func ExamplePop() {
	var list = []int{1, 2, 3}

	v, ok := slicekit.Pop(&list)
	_ = ok   // true
	_ = v    // 3
	_ = list // []int{1, 2}
}

func ExamplePop_onEmpty() {
	var list = []string{}

	v, ok := slicekit.Pop(&list)
	_ = ok   // false
	_ = v    // ""
	_ = list // []string{}
}

func ExamplePop_onNil() {
	var list []byte

	v, ok := slicekit.Pop(&list)
	_ = ok   // false
	_ = v    // 0
	_ = list // ([]byte)(nil)
}

func TestPop(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("nil slice pointer", func(t *testcase.T) {
		v, ok := slicekit.Pop[[]string](nil)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("nil slice", func(t *testcase.T) {
		var list []string
		v, ok := slicekit.Pop(&list)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("empty slice", func(t *testcase.T) {
		v, ok := slicekit.Pop(&[]string{})
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("len 1", func(t *testcase.T) {
		exp := t.Random.Int()
		list := []int{exp}
		got, ok := slicekit.Pop(&list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Empty(t, list)
	})

	s.Test("len 1+", func(t *testcase.T) {
		var (
			list      []int
			remaining []int
		)
		t.Random.Repeat(1, 7, func() {
			v := t.Random.Int()
			list = append(list, v)
			remaining = append(remaining, v)
		})
		exp := t.Random.Int()
		list = append(list, exp)
		got, ok := slicekit.Pop(&list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Equal(t, list, remaining)
	})
}

func ExampleShift() {
	var list = []int{1, 2, 3}

	v, ok := slicekit.Shift(&list)
	_ = ok   // true
	_ = v    // 1
	_ = list // []int{2, 3}
}

func ExampleShift_onEmpty() {
	var list = []string{}

	v, ok := slicekit.Shift(&list)
	_ = ok   // false
	_ = v    // ""
	_ = list // []string{}
}

func ExampleShift_onNil() {
	var list []byte

	v, ok := slicekit.Shift(&list)
	_ = ok   // false
	_ = v    // 0
	_ = list // ([]byte)(nil)
}

func TestShift(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("nil slice pointer", func(t *testcase.T) {
		v, ok := slicekit.Shift[[]string](nil)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("nil slice", func(t *testcase.T) {
		var list []string
		v, ok := slicekit.Shift(&list)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("empty slice", func(t *testcase.T) {
		v, ok := slicekit.Shift(&[]string{})
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("len 1", func(t *testcase.T) {
		exp := t.Random.Int()
		list := []int{exp}
		got, ok := slicekit.Shift(&list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Empty(t, list)
	})

	s.Test("len 1+", func(t *testcase.T) {
		var (
			list      []int
			remaining []int
		)
		exp := t.Random.Int()
		list = append(list, exp)
		t.Random.Repeat(1, 7, func() {
			v := t.Random.Int()
			list = append(list, v)
			remaining = append(remaining, v)
		})
		got, ok := slicekit.Shift(&list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Equal(t, list, remaining)
	})
}

func ExampleUnshift() {
	var list []string
	_ = list // ([]string)(nil)
	slicekit.Unshift(&list, "foo")
	_ = list // []string{"foo"}
	slicekit.Unshift(&list, "bar")
	_ = list // []string{"bar", "foo"}
	slicekit.Unshift(&list, "baz", "qux")
	_ = list // []string{"baz", "qux", "bar", "foo"}
}

func TestUnshift(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("nil slice pointer", func(t *testcase.T) {
		assert.Panic(t, func() {
			slicekit.Unshift[[]string](nil, "")
		})
	})

	s.Test("nil slice", func(t *testcase.T) {
		var list []string
		exp := t.Random.String()
		slicekit.Unshift(&list, exp)
		assert.Equal(t, []string{exp}, list)
	})

	s.Test("empty slice", func(t *testcase.T) {
		var list = []string{}
		exp := t.Random.String()
		slicekit.Unshift(&list, exp)
		assert.Equal(t, []string{exp}, list)
	})

	s.Test("len 1 - unshift 1", func(t *testcase.T) {
		og := t.Random.Int()
		n := t.Random.Int()
		list := []int{og}
		slicekit.Unshift(&list, n)
		assert.Equal(t, list, []int{n, og})
	})

	s.Test("len 1 - unshift 2", func(t *testcase.T) {
		og := t.Random.Int()
		n1 := t.Random.Int()
		n2 := t.Random.Int()
		list := []int{og}
		exp := []int{n1, n2, og}
		slicekit.Unshift(&list, n1, n2)
		assert.Equal(t, list, exp)
	})

	s.Test("len 1+", func(t *testcase.T) {
		var (
			list []int
			exp  []int
		)
		n := t.Random.Int()
		exp = append(exp, n)
		t.Random.Repeat(1, 7, func() {
			v := t.Random.Int()
			list = append(list, v)
			exp = append(exp, v)
		})
		slicekit.Unshift(&list, n)
		assert.Equal(t, list, exp)
	})
}

func ExampleLast() {
	var list = []int{1, 2, 3}
	last, ok := slicekit.Last(list)
	_ = ok   // true
	_ = last // 3
}

func ExampleLast_onEmpty() {
	var list = []string{}
	last, ok := slicekit.Last(list)
	_ = ok   // false
	_ = last // ""
}

func ExampleLast_onNil() {
	var list []byte
	last, ok := slicekit.Last(list)
	_ = ok   // false
	_ = last // 0
}

func TestLast(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("nil slice", func(t *testcase.T) {
		v, ok := slicekit.Last(([]string)(nil))
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("empty slice", func(t *testcase.T) {
		v, ok := slicekit.Last([]string{})
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("len 1", func(t *testcase.T) {
		exp := t.Random.Int()
		list := []int{exp}
		got, ok := slicekit.Last(list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.NotEmpty(t, list)
	})

	s.Test("len 1+", func(t *testcase.T) {
		var (
			list []int
			og   []int
		)
		t.Random.Repeat(1, 7, func() {
			v := t.Random.Int()
			list = append(list, v)
			og = append(og, v)
		})
		exp := t.Random.Int()
		list = append(list, exp)
		og = append(og, exp)
		got, ok := slicekit.Last(list)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Equal(t, list, og)
	})
}

func TestInsert(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		og = testcase.Let(s, func(t *testcase.T) []string {
			return []string{"foo", "bar", "baz"}
		})
		slice = testcase.Let(s, func(t *testcase.T) *[]string {
			var s = slicekit.Clone(og.Get(t))
			return &s
		})
		index  = testcase.Let[int](s, nil)
		values = testcase.Let(s, func(t *testcase.T) []string {
			return random.Slice(t.Random.IntBetween(3, 5), t.Random.String, random.UniqueValues)
		})
	)
	act := func(t *testcase.T) bool {
		return slicekit.Insert(slice.Get(t), index.Get(t), values.Get(t)...)
	}

	s.When("input slice is empty/nil", func(s *testcase.Spec) {
		slice.Let(s, func(t *testcase.T) *[]string {
			var s []string
			if t.Random.Bool() {
				s = []string{}
			}
			return &s
		})

		index.LetValue(s, 0)

		s.Then("it will add the values to it", func(t *testcase.T) {
			assert.True(t, act(t))

			assert.Equal(t, *slice.Get(t), values.Get(t))
		})
	})

	s.When("index is at zero", func(s *testcase.Spec) {
		index.LetValue(s, 0)

		s.Then("it will act as unshift", func(t *testcase.T) {
			assert.True(t, act(t))

			var exp []string
			exp = append(exp, values.Get(t)...)
			exp = append(exp, og.Get(t)...)
			assert.Equal(t, *slice.Get(t), exp)
		})
	})

	s.When("index is pointing somewhere inside the slice", func(s *testcase.Spec) {
		index.LetValue(s, 1)

		s.Then("it insert the values to the posistion", func(t *testcase.T) {
			assert.True(t, act(t))

			var exp []string
			exp = append(exp, og.Get(t)[0])
			exp = append(exp, values.Get(t)...)
			exp = append(exp, og.Get(t)[1:]...)
			assert.Equal(t, *slice.Get(t), exp)
		})
	})

	s.When("index is pointing to next index number of the slice", func(s *testcase.Spec) {
		index.Let(s, func(t *testcase.T) int {
			return len(*slice.Get(t))
		})

		s.Then("it will append the new values at the end of the slice", func(t *testcase.T) {
			assert.True(t, act(t))

			var exp []string
			exp = append(exp, og.Get(t)...)
			exp = append(exp, values.Get(t)...)
			assert.Equal(t, *slice.Get(t), exp)
		})
	})

	s.When("index is referencing the last element through using -1", func(s *testcase.Spec) {
		index.LetValue(s, -1)

		s.Then("it will insert the values at the last index position, just before/in-place of the last element", func(t *testcase.T) {
			assert.True(t, act(t))

			lastIndex := len(og.Get(t)) - 1
			var exp []string
			exp = append(exp, og.Get(t)[0:lastIndex]...)
			exp = append(exp, values.Get(t)...)
			exp = append(exp, og.Get(t)[lastIndex])

			assert.Equal(t, exp, *slice.Get(t))
		})
	})

	s.When("index is bigger than the input slice", func(s *testcase.Spec) {
		index.Let(s, func(t *testcase.T) int {
			return len(og.Get(t)) + t.Random.IntBetween(1, 7)
		})

		s.Then("it will report that insertion is not possible due to being too much out of bound", func(t *testcase.T) {
			assert.False(t, act(t))

			assert.Equal(t, og.Get(t), *slice.Get(t))
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		var makeElem = func() string {
			return t.Random.HexN(5)
		}
		for i := range 10 {
			i += 10 // offset
			vs := random.Slice(i, makeElem)
			index := t.Random.IntN(len(vs))
			nvs := random.Slice(t.Random.IntBetween(3, 5), makeElem)
			got := slicekit.Clone(vs)
			slicekit.Insert(&got, index, nvs...)
			assert.Contains(t, got, vs)
			assert.Contains(t, got, nvs)
			assert.Equal(t, nvs, got[index:index+len(nvs)])
			assert.Equal(t, vs[:index], got[:index])
			assert.Equal(t, vs[index:], got[index+len(nvs):])
			// assert.Equal(t, got[:index], vs[:index])
		}
	})
}

func TestAnyOf(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		t.Run("matching element exists", func(t *testing.T) {
			input := []int{1, 2, 3, 4, 5}
			result := slicekit.AnyOf(input, func(v int) bool { return v%2 == 0 })
			assert.True(t, result)
		})

		t.Run("multiple matching element exists", func(t *testing.T) {
			input := []int{1, 2, 3, 4, 5}
			result := slicekit.AnyOf(input, func(v int) bool { return true })
			assert.True(t, result)
		})

		t.Run("no matching element", func(t *testing.T) {
			input := []int{1, 3, 5, 7}
			result := slicekit.AnyOf(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, result)
		})

		t.Run("empty slice", func(t *testing.T) {
			input := []int{}
			result := slicekit.AnyOf(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, result)
		})
	})

	t.Run("edge cases", func(t *testing.T) {
		t.Run("single element matching", func(t *testing.T) {
			input := []int{2}
			result := slicekit.AnyOf(input, func(v int) bool { return v%2 == 0 })
			assert.True(t, result)
		})

		t.Run("single element non-matching", func(t *testing.T) {
			input := []int{3}
			result := slicekit.AnyOf(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, result)
		})
	})
}

func ExampleFind() {
	type Person struct {
		Name     string
		Birthday time.Time
	}

	var ps []Person

	person, ok := slicekit.Find(ps, func(p Person) bool {
		return p.Birthday.After(time.Date(2000, 1, 1, 12, 00, 00, 00, time.UTC))
	})

	_, _ = person, ok
}

func TestFind(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		t.Run("matching element exists", func(t *testing.T) {
			input := []int{1, 2, 3, 4, 5}
			val, ok := slicekit.Find(input, func(v int) bool { return v == 2 })
			assert.True(t, ok)
			assert.Equal(t, val, 2)
		})

		t.Run("multiple matching element exists, then first returned", func(t *testing.T) {
			input := []int{1, 2, 3, 4, 5}
			val, ok := slicekit.Find(input, func(v int) bool { return true })
			assert.True(t, ok)
			assert.Equal(t, val, 1)
		})

		t.Run("no matching element", func(t *testing.T) {
			input := []int{1, 3, 5, 7}
			val, ok := slicekit.Find(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, ok)
			assert.Empty(t, val)
		})

		t.Run("empty slice", func(t *testing.T) {
			input := []int{}
			val, ok := slicekit.Find(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, ok)
			assert.Empty(t, val)
		})
	})

	t.Run("edge cases", func(t *testing.T) {
		t.Run("single element matching", func(t *testing.T) {
			input := []int{2}
			val, ok := slicekit.Find(input, func(v int) bool { return v%2 == 0 })
			assert.True(t, ok)
			assert.Equal(t, val, 2)
		})

		t.Run("single element non-matching", func(t *testing.T) {
			input := []int{3}
			val, ok := slicekit.Find(input, func(v int) bool { return v%2 == 0 })
			assert.False(t, ok)
			assert.Empty(t, val)
		})
	})
}

func ExampleGroupBy() {
	vs := []int{1, 2, 3, 4, 5}

	groups := slicekit.GroupBy(vs, func(n int) string {
		if n%2 == 0 {
			return "even"
		}
		return "odd"
	})

	_ = groups
}

func TestGroupBy(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		assert.Nil(t, slicekit.GroupBy[[]int, int](nil, func(v int) int { return 0 }))
	})

	t.Run("empty slice", func(t *testing.T) {
		vs := []int{}
		g := slicekit.GroupBy(vs, func(int) int { return int(time.Now().UnixNano()) })
		assert.Empty(t, g)
	})

	t.Run("nil group by func", func(t *testing.T) {
		assert.Panic(t, func() {
			_ = slicekit.GroupBy[[]int, int]([]int{1, 2, 3}, nil)
		})
	})

	t.Run("E2E", func(t *testing.T) {
		in := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		exp := map[string][]int{
			"odd":  {1, 3, 5, 7, 9},
			"even": {0, 2, 4, 6, 8},
		}
		got := slicekit.GroupBy(in, func(n int) string {
			if n%2 == 0 {
				return "even"
			}
			return "odd"
		})
		assert.Equal(t, got, exp)
	})
}

func TestSortBy(t *testing.T) {
	A := []int{1, 3, 2}
	slicekit.SortBy(A, func(a, b int) bool { return a < b })
	assert.Equal(t, A, []int{1, 2, 3})

	B := []string{"foo", "bar", "baz"}
	slicekit.SortBy(B, func(a, b string) bool { return a < b })
	assert.Equal(t, B, []string{"bar", "baz", "foo"})
}

func TestFirst(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		_, ok := slicekit.First[[]string](nil)
		assert.False(t, ok)
	})
	t.Run("empty", func(t *testing.T) {
		_, ok := slicekit.First([]string{})
		assert.False(t, ok)
	})
	t.Run("non empty", func(tt *testing.T) {
		t := testcase.NewT(tt)
		exp := t.Random.String()

		var in = []string{exp}
		t.Random.Repeat(0, 3, func() {
			in = append(in, t.Random.String())
		})

		got, ok := slicekit.First(in)
		assert.True(t, ok)
		assert.Equal(t, exp, got)
	})
}

func TestPopAt(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("nil slice pointer", func(t *testcase.T) {
		v, ok := slicekit.PopAt[[]string](nil, 0)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("nil slice", func(t *testcase.T) {
		var list []string
		v, ok := slicekit.PopAt(&list, t.Random.IntBetween(0, 100))
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("empty slice", func(t *testcase.T) {
		v, ok := slicekit.PopAt(&[]string{}, t.Random.IntBetween(0, 100))
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	s.Test("len 1 with index 0", func(t *testcase.T) {
		exp := t.Random.Int()
		list := []int{exp}
		got, ok := slicekit.PopAt(&list, 0)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Empty(t, list)
	})

	s.Test("non empty but negative index, then it will count backwards starting with -1 being the last", func(t *testcase.T) {
		exp := t.Random.Int()
		first := t.Random.Int()
		list := []int{first, exp}
		got, ok := slicekit.PopAt(&list, -1)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.NotEmpty(t, list)
		assert.Equal(t, []int{first}, list)
	})

	s.Test("len 1 with out of index", func(t *testcase.T) {
		exp := t.Random.Int()
		list := []int{exp}
		got, ok := slicekit.PopAt(&list, t.Random.IntBetween(1, 100))
		assert.False(t, ok)
		assert.Empty(t, got)
		assert.Equal(t, list, []int{exp})
	})

	s.Test("len 1+ with last index", func(t *testcase.T) {
		var (
			list      []int
			remaining []int
		)
		t.Random.Repeat(1, 7, func() {
			v := t.Random.Int()
			list = append(list, v)
			remaining = append(remaining, v)
		})
		exp := t.Random.Int()
		list = append(list, exp)
		got, ok := slicekit.PopAt(&list, len(list)-1)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Equal(t, list, remaining)
	})

	s.Test("len 1+ with an index pointing to a middle", func(t *testcase.T) {
		var (
			exp   = t.Random.Int()
			first = t.Random.Int()
			last  = t.Random.Int()
			list  = []int{first, exp, last}
		)

		got, ok := slicekit.PopAt(&list, 1)
		assert.True(t, ok)
		assert.Equal(t, got, exp)
		assert.Equal(t, list, []int{first, last})
	})
}

func ExampleIterReverse() {
	var vs = []int{1, 2, 3}

	for _, v := range slicekit.IterReverse(vs) {
		_ = v // 3 -> 2 -> 1
	}
}

func TestIterReverse_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		slc := []string{}
		got := iterkit.Collect(iterkit.ToV(slicekit.IterReverse(slc)))
		assert.Equal(t, slc, got)
	})

	s.Test("not empty values", func(t *testcase.T) {
		slc := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		got := iterkit.Collect(iterkit.Reverse(iterkit.ToV(slicekit.IterReverse(slc))))
		assert.Equal(t, slc, got)
	})

	s.Test("not empty index", func(t *testcase.T) {
		slc := []int{1, 2, 3}
		exp := []iterkit.KV[int, int]{
			{K: 2, V: 3},
			{K: 1, V: 2},
			{K: 0, V: 1},
		}
		got := iterkit.Collect2KV(slicekit.IterReverse(slc))
		assert.Equal(t, exp, got)
	})
}

func TestDelete(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		og = testcase.Let(s, func(t *testcase.T) []string {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.String, random.UniqueValues)
		})
		slice = testcase.Let(s, func(t *testcase.T) *[]string {
			var s = slicekit.Clone(og.Get(t))
			return &s
		})
		index = testcase.Let[int](s, nil)
	)
	act := func(t *testcase.T) bool {
		return slicekit.Delete(slice.Get(t), index.Get(t))
	}

	s.When("input slice is empty/nil", func(s *testcase.Spec) {
		slice.Let(s, func(t *testcase.T) *[]string {
			var s []string
			if t.Random.Bool() {
				s = []string{}
			}
			return &s
		})

		index.Let(s, let.IntB(s, 0, 3).Get)

		s.Then("unsuccess is reported", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("index is pointing somewhere inside the slice", func(s *testcase.Spec) {
		index.Let(s, func(t *testcase.T) int {
			return t.Random.IntN(len(og.Get(t)))
		})

		s.Then("it deletes the value at the index", func(t *testcase.T) {
			assert.True(t, act(t))

			var exp []string
			exp = append(exp, og.Get(t)[:index.Get(t)]...)

			if len(og.Get(t))-1 != index.Get(t) { // if not Pop case, add the remaining ones
				exp = append(exp, og.Get(t)[index.Get(t)+1:]...)
			}

			assert.Equal(t, exp, *slice.Get(t))
		})

		s.And("if the index points to the last element", func(s *testcase.Spec) {
			index.Let(s, func(t *testcase.T) int {
				if t.Random.Bool() {
					t.Log("given last element identified with -1")
					return -1
				}

				i := len(og.Get(t)) - 1
				assert.NotEqual(t, i, -1, "incorrect test setup")
				return i
			})

			s.Then("delete act like deque pop", func(t *testcase.T) {
				assert.True(t, act(t))
				lastIndex := len(og.Get(t)) - 1
				exp := og.Get(t)[:lastIndex]
				assert.Equal(t, exp, *slice.Get(t))
			})
		})

		s.And("if the index points to the first element", func(s *testcase.Spec) {
			index.LetValue(s, 0)

			s.Then("delete act like deque shift", func(t *testcase.T) {
				assert.True(t, act(t))
				exp := og.Get(t)[1:]
				assert.Equal(t, exp, *slice.Get(t))
			})
		})
	})

	s.When("index is bigger than the input slice", func(s *testcase.Spec) {
		index.Let(s, func(t *testcase.T) int {
			return len(og.Get(t)) + t.Random.IntBetween(1, 7)
		})

		s.Then("it will report that insertion is not possible due to being too much out of bound", func(t *testcase.T) {
			assert.False(t, act(t))

			assert.Equal(t, og.Get(t), *slice.Get(t))
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		var makeElem = func() string {
			return t.Random.HexN(5)
		}
		for i := range 10 {
			i += 10 // offset
			vs := random.Slice(i, makeElem)
			index := t.Random.IntN(len(vs))
			nvs := random.Slice(t.Random.IntBetween(3, 5), makeElem)
			got := slicekit.Clone(vs)
			slicekit.Insert(&got, index, nvs...)
			assert.Contains(t, got, vs)
			assert.Contains(t, got, nvs)
			assert.Equal(t, nvs, got[index:index+len(nvs)])
			assert.Equal(t, vs[:index], got[:index])
			assert.Equal(t, vs[index:], got[index+len(nvs):])
		}
	})
}

func ExampleResolveIndex() {
	var s = []string{"foo", "bar", "baz"}
	_, _ = slicekit.ResolveIndex(len(s), 1)  // 1, true
	_, _ = slicekit.ResolveIndex(len(s), 2)  // 2, true
	_, _ = slicekit.ResolveIndex(len(s), -1) // 2, true
	_, _ = slicekit.ResolveIndex(len(s), 3)  // 3, false
}

func TestResolveIndex(t *testing.T) {
	var s = []string{"foo", "bar", "baz"}
	rnd := random.New(random.CryptoSeed{})

	t.Run("pos", func(t *testing.T) {
		for exp := range s {
			got, ok := slicekit.ResolveIndex(len(s), exp)
			assert.True(t, ok)
			assert.Equal(t, exp, got)
		}
	})
	t.Run("pos - out of bound", func(t *testing.T) {
		i := len(s) + rnd.IntBetween(0, 100)
		got, ok := slicekit.ResolveIndex(len(s), i)
		assert.False(t, ok)
		assert.Equal(t, i, got)
	})

	t.Run("neg", func(t *testing.T) {
		for exp := range s {
			// 0 - 3 = -3
			// 1 - 3 = -2
			// 2 - 3 = -1
			var negIndex = exp - len(s)
			got, ok := slicekit.ResolveIndex(len(s), negIndex)
			assert.True(t, ok)
			assert.Equal(t, exp, got)
		}
	})

	t.Run("neg - out of bound", func(t *testing.T) {
		var negIndex = -len(s) - rnd.IntBetween(1, 100)
		got, ok := slicekit.ResolveIndex(len(s), negIndex)
		assert.False(t, ok)
		assert.Equal(t, negIndex, got)
	})
}

func Test_sequence(t *testing.T) {
	dscontract.Sequence(func(tb testing.TB) ds.Sequence[string] {
		return &Sequence[string]{}
	}).Test(t)
}

// Sequence is a sample implementation using slicekit helper functions
// to implement the Sequence data structure.
type Sequence[T any] []T

func (seq *Sequence[T]) Lookup(index int) (T, bool) {
	return slicekit.Lookup(*seq, index)
}

func (seq *Sequence[T]) Set(index int, val T) bool {
	return slicekit.Set(*seq, index, val)
}

func (seq *Sequence[T]) Insert(index int, vs ...T) bool {
	return slicekit.Insert(seq, index, vs...)
}

func (seq *Sequence[T]) Delete(index int) bool {
	return slicekit.Delete(seq, index)
}

func (seq *Sequence[T]) Append(vs ...T) {
	*seq = append(*seq, vs...)
}

func (seq *Sequence[T]) ToSlice() []T {
	return *seq
}

func (seq *Sequence[T]) Values() iter.Seq[T] {
	return iterkit.FromSlice(*seq)
}

func (seq *Sequence[T]) Len() int {
	return len(*seq)
}

func TestContains(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("contains a value in a non-empty slice", func(t *testcase.T) {
		sVS := random.Slice(t.Random.IntBetween(3, 7), t.Random.String)
		assert.True(t, slicekit.Contains(sVS, random.Pick(t.Random, sVS...)))

		iVS := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		assert.True(t, slicekit.Contains(iVS, random.Pick(t.Random, iVS...)))
	})

	s.Test("contains the first element", func(t *testcase.T) {
		assert.True(t, slicekit.Contains([]string{"foo", "bar", "baz"}, "foo"))

		vs := random.Slice(t.Random.IntBetween(3, 7), t.Random.String)
		assert.True(t, slicekit.Contains(vs, vs[0]))
	})

	s.Test("contains the last element", func(t *testcase.T) {
		assert.True(t, slicekit.Contains([]string{"foo", "bar", "baz"}, "baz"))

		vs := random.Slice(t.Random.IntBetween(3, 7), t.Random.String)
		assert.True(t, slicekit.Contains(vs, vs[len(vs)-1]))
	})

	s.Test("does not contain a value that is not in the slice", func(t *testcase.T) {
		assert.False(t, slicekit.Contains([]string{"foo", "bar", "baz"}, "qux"))
		assert.False(t, slicekit.Contains([]int{7, 42, 128}, 32))

		vs := random.Slice(t.Random.IntBetween(3, 7), t.Random.String)
		oth := random.Unique(t.Random.String, vs...)
		assert.False(t, slicekit.Contains(vs, oth))
	})

	s.Test("does not contain any value in an empty slice", func(t *testcase.T) {
		assert.False(t, slicekit.Contains([]string{}, "foo"))
		assert.False(t, slicekit.Contains([]string{}, ""))
	})

	s.Test("does not contain any value in a nil slice", func(t *testcase.T) {
		assert.False(t, slicekit.Contains([]string(nil), "foo"))
	})

	s.Test("duplicate values are still contained", func(t *testcase.T) {
		// Membership is a logical predicate: if the value is present at
		// any index, Contains returns true regardless of how many times
		// it appears.
		assert.True(t, slicekit.Contains([]string{"foo", "foo", "bar"}, "foo"))
	})

	s.Test("time.Time values that are Equal but not == are matched", func(t *testcase.T) {
		// time.Time is a comparable type but its == operator is strict:
		// two time.Time values that denote the same instant in different
		// *Location zones are NOT equal under == but ARE equal under
		// .Equal. The desired contract for Contains is to honour
		// predicate.Equatable[T].Equal, mirroring ContainsExactly.
		//
		// This subtest is the spec for that contract. Until Contains
		// is updated to dispatch to Equal, the Equal-but-not-==
		// assertion below will fail and surface the gap.
		utc := time.UTC
		fixed := time.FixedZone("fixed-utc", 0)
		when := time.Date(2024, 1, 2, 0, 0, 0, 0, utc)
		whenAlt := time.Date(2024, 1, 2, 0, 0, 0, 0, fixed)
		otherInstant := time.Date(2024, 6, 15, 0, 0, 0, 0, utc)

		// Sanity-check the precondition that makes this test meaningful.
		if when == whenAlt {
			t.Fatalf("test precondition broken: when == whenAlt under ==")
		}
		if !when.Equal(whenAlt) {
			t.Fatalf("test precondition broken: !when.Equal(whenAlt)")
		}
		if when.Equal(otherInstant) {
			t.Fatalf("test precondition broken: when.Equal(otherInstant)")
		}

		// Exact == match.
		assert.True(t, slicekit.Contains([]time.Time{when}, when))

		// Equal-but-not-== match: should be found under the Equatable contract.
		assert.True(t, slicekit.Contains([]time.Time{when}, whenAlt))

		// A genuinely different value (different instant) must still differ.
		assert.False(t, slicekit.Contains([]time.Time{when}, otherInstant))
	})

	s.Test("user-defined Equatable", func(t *testcase.T) {
		// namedEntry.Equal compares only the name field, so two entries
		// with the same name but different ids are Equal (but not ==).
		// The desired contract is that Contains dispatches to Equal and
		// finds them. Until the implementation is updated, the
		// "an Equal match is found" assertion below will fail and surface
		// the gap.
		vs := []namedEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}

		assert.True(t, slicekit.Contains(vs, namedEntry{id: 1, name: "foo"}),
			"an exact == match is found")
		assert.True(t, slicekit.Contains(vs, namedEntry{id: 42, name: "foo"}),
			"an Equal match is found")

		// Negative case: a needle that is neither == nor Equal to any
		// element in the slice is not found.
		needle := namedEntry{id: 99, name: "baz"}
		assert.False(t, slicekit.Contains(vs, needle))
	})

	s.Test("user-defined Comparable", func(t *testcase.T) {
		// comparableEntry implements predicate.Comparable[T] but NOT
		// predicate.Equatable[T]. Compare returns 0 for entries with
		// the same name, so a needle with a different id but the same
		// name should be found when Contains dispatches to Compare.
		//
		// This subtest proves the dispatch reaches for Compare
		// specifically. Until the implementation dispatches to
		// Comparable, the positive assertion below will fail and
		// surface the gap.
		vs := []comparableEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}

		assert.True(t, slicekit.Contains(vs, comparableEntry{id: 1, name: "foo"}),
			"an exact == match is found")
		assert.True(t, slicekit.Contains(vs, comparableEntry{id: 42, name: "foo"}),
			"an Equal-under-Compare match is found")

		// Negative case: a needle that is not Compare-equal to any
		// element in the slice is not found.
		assert.False(t, slicekit.Contains(vs, comparableEntry{id: 99, name: "baz"}))
	})

	s.Test("user-defined ComparableShort", func(t *testcase.T) {
		// comparableShortEntry implements predicate.ComparableShort[T]
		// but NOT predicate.Equatable[T]. Cmp returns 0 for entries
		// with the same name, so a needle with a different id but the
		// same name should be found when Contains dispatches to Cmp.
		//
		// This subtest proves the dispatch reaches for Cmp
		// specifically. Until the implementation dispatches to
		// ComparableShort, the positive assertion below will fail and
		// surface the gap.
		vs := []comparableShortEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}

		assert.True(t, slicekit.Contains(vs, comparableShortEntry{id: 1, name: "foo"}),
			"an exact == match is found")
		assert.True(t, slicekit.Contains(vs, comparableShortEntry{id: 42, name: "foo"}),
			"an Equal-under-Cmp match is found")

		// Negative case: a needle that is not Cmp-equal to any
		// element in the slice is not found.
		assert.False(t, slicekit.Contains(vs, comparableShortEntry{id: 99, name: "baz"}))
	})

	s.Test("NaN", func(t *testcase.T) {
		var nan = math.NaN()
		assert.True(t, slicekit.Contains([]float64{42.24, nan}, nan))
		assert.True(t, slicekit.Contains([]float64{42.24, nan}, 42.24))
	})
}

func TestContainsExactly(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("1:1 match non-empty", func(t *testcase.T) {
		assert.True(t, slicekit.ContainsExactly([]string{"foo", "bar", "baz"}, []string{"foo", "bar", "baz"}))
	})

	s.Test("1:1 match, but different order", func(t *testcase.T) {
		assert.True(t, slicekit.ContainsExactly([]string{"foo", "bar", "baz"}, []string{"bar", "baz", "foo"}))
	})

	s.Test("1:1 match, empty", func(t *testcase.T) {
		assert.True(t, slicekit.ContainsExactly([]string{}, []string{}))
		assert.True(t, slicekit.ContainsExactly([]string{}, nil))
		assert.True(t, slicekit.ContainsExactly(nil, []string{}))
	})

	s.Test("no match, length", func(t *testcase.T) {
		assert.False(t, slicekit.ContainsExactly([]string{"foo", "bar", "baz"}, []string{"foo", "bar", "baz", "baz"}))
	})

	s.Test("no match, different element", func(t *testcase.T) {
		assert.False(t, slicekit.ContainsExactly([]string{"foo", "bar", "baz"}, []string{"foo", "qux", "baz"}))
	})

	s.Test("no match, different content", func(t *testcase.T) {
		assert.False(t, slicekit.ContainsExactly(
			[]string{"foo", "foo", "bar"},
			[]string{"foo", "bar", "bar"},
		))
	})

	s.Test("NaN float values are compared correctly", func(t *testcase.T) {
		var nan = math.NaN()

		assert.True(t, slicekit.ContainsExactly([]float64{nan}, []float64{nan}))
		assert.True(t, slicekit.ContainsExactly([]float64{42.24, nan}, []float64{nan, 42.24}))
		// [1.0] and [NaN] clearly do not contain the same values,
		// yet the current implementation reports them as equal.
		//
		// Root cause: the values are used as map keys, but NaN != NaN in Go,
		// so a NaN key can never be found again on lookup. The decrement guard
		// `counts[v]--; if counts[v] < 0` therefore never trips for NaN, and a
		// fresh (invisible) key is inserted on every access instead.

		assert.False(t, slicekit.ContainsExactly([]float64{1.0}, []float64{nan}))

		// Same defect with a duplicated NaN: these multisets differ.
		assert.False(t, slicekit.ContainsExactly(
			[]float64{nan, 1.0},
			[]float64{nan, nan},
		))

		// The contract is order-independent (hence symmetric) value comparison,
		// but with NaN the answer depends on the argument order.
		assert.True(t, slicekit.ContainsExactly([]float64{1.0}, []float64{nan}) ==
			slicekit.ContainsExactly([]float64{nan}, []float64{1.0}))
	})

	s.Test("Inf float values are compared correctly", func(t *testcase.T) {
		var posInf = math.Inf(+1)
		var negInf = math.Inf(-1)

		assert.True(t, posInf == posInf)
		assert.True(t, negInf == negInf)

		// +Inf == +Inf and -Inf == -Inf in Go, so the generic == path
		// behaves correctly for these values; the float64 specialisation
		// must preserve that.
		assert.True(t, slicekit.ContainsExactly(
			[]float64{posInf, 1.0},
			[]float64{1.0, posInf},
		))
		assert.True(t, slicekit.ContainsExactly(
			[]float64{negInf, negInf, 2.0},
			[]float64{negInf, 2.0, negInf},
		))

		// +Inf and -Inf are distinct.
		assert.False(t, slicekit.ContainsExactly(
			[]float64{posInf},
			[]float64{negInf},
		))

		// Inf counts must match.
		assert.False(t, slicekit.ContainsExactly(
			[]float64{posInf, 1.0},
			[]float64{1.0, 1.0},
		))

		// Symmetric across argument order.
		assert.True(t, slicekit.ContainsExactly(
			[]float64{posInf, negInf},
			[]float64{negInf, posInf},
		) == slicekit.ContainsExactly(
			[]float64{negInf, posInf},
			[]float64{posInf, negInf},
		))
	})

	s.Test("time.Time values that are Equal but not == are matched", func(t *testcase.T) {
		// time.Time is a comparable type but its == operator is strict:
		// two time.Time values that denote the same instant in different
		// *Location zones are NOT equal under == but ARE equal under .Equal.
		//
		// The two locations below represent the same wall-clock instant
		// (00:00 UTC on 2024-01-02) but use different *Location pointers.
		utc := time.UTC
		fixed := time.FixedZone("fixed-utc", 0)

		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, utc)
		t1Alt := time.Date(2024, 1, 2, 0, 0, 0, 0, fixed)
		t2 := time.Date(2024, 1, 2, 1, 0, 0, 0, utc)
		t2Alt := time.Date(2024, 1, 2, 1, 0, 0, 0, fixed)

		// Sanity-check the precondition that makes this test meaningful.
		assert.True(t, !t1.Equal(t1Alt) == false) // tautology, just to anchor
		assert.True(t, t1.Equal(t1Alt))
		assert.True(t, !t1.Equal(t2Alt))
		if t1 == t1Alt {
			t.Fatalf("test precondition broken: t1 == t1Alt under ==")
		}

		// Slices whose only pairwise difference is the *Location pointer
		// should be considered to contain the same values.
		assert.True(t, slicekit.ContainsExactly(
			[]time.Time{t1, t2},
			[]time.Time{t1Alt, t2Alt},
		))

		// And the order must not matter.
		assert.True(t, slicekit.ContainsExactly(
			[]time.Time{t1, t2},
			[]time.Time{t2Alt, t1Alt},
		))

		// Symmetric: the Equal-based comparison must not depend on which
		// side is the receiver.
		assert.True(t, slicekit.ContainsExactly(
			[]time.Time{t1, t2},
			[]time.Time{t1Alt, t2Alt},
		) == slicekit.ContainsExactly(
			[]time.Time{t1Alt, t2Alt},
			[]time.Time{t1, t2},
		))

		// A genuinely different value (different instant) must still differ.
		assert.False(t, slicekit.ContainsExactly(
			[]time.Time{t1, t2},
			[]time.Time{t1Alt, t1Alt},
		))
	})

	s.Test("user-defined type with Equal method", func(t *testcase.T) {
		// A custom comparable type that defines a custom Equal that is
		// broader than ==. The id field is intentionally ignored by
		// Equal so that two entries with the same name but different
		// ids are still considered equal.
		vs := []namedEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}
		oth := []namedEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
		}
		assert.True(t, slicekit.ContainsExactly(vs, oth))

		vs = []namedEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "foo"},
			{id: 3, name: "bar"},
		}
		oth = []namedEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
		}
		assert.True(t, slicekit.ContainsExactly(vs, oth))

		vs = []namedEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "foo"},
			{id: 3, name: "bar"},
		}
		oth = []namedEntry{
			{id: 1, name: "foo"},
			{id: 3, name: "bar"},
			{id: 4, name: "bar"},
		}
		assert.False(t, slicekit.ContainsExactly(vs, oth))
	})

	s.Test("user-defined Comparable type", func(t *testcase.T) {
		// comparableEntry implements predicate.Comparable[T] but NOT
		// predicate.Equatable[T]. Compare returns 0 for entries with
		// the same name, so two values with different ids but the
		// same name are equal under Compare but not under ==.
		//
		// This subtest proves the dispatch reaches for Compare
		// specifically: if ContainsExactly were using == (or Even if
		// it implemented an Equatable fallback), the matching would
		// fail here because the entries have different ids.
		//
		// Until the implementation dispatches to Comparable, this test
		// will fail and surface the gap.
		vs := []comparableEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}
		oth := []comparableEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
		}
		assert.True(t, slicekit.ContainsExactly(vs, oth))

		// Compare-equal entries with different counts must still differ.
		vs = []comparableEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "foo"},
			{id: 3, name: "bar"},
		}
		oth = []comparableEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
			{id: 99, name: "bar"},
		}
		assert.False(t, slicekit.ContainsExactly(vs, oth))
	})

	s.Test("user-defined ComparableShort type", func(t *testcase.T) {
		// comparableShortEntry implements predicate.ComparableShort[T]
		// but NOT predicate.Equatable[T]. Cmp returns 0 for entries
		// with the same name, so two values with different ids but
		// the same name are equal under Cmp but not under ==.
		//
		// This subtest proves the dispatch reaches for Cmp
		// specifically. Until the implementation dispatches to
		// ComparableShort, this test will fail and surface the gap.
		vs := []comparableShortEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "bar"},
		}
		oth := []comparableShortEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
		}
		assert.True(t, slicekit.ContainsExactly(vs, oth))

		// Cmp-equal entries with different counts must still differ.
		vs = []comparableShortEntry{
			{id: 1, name: "foo"},
			{id: 2, name: "foo"},
			{id: 3, name: "bar"},
		}
		oth = []comparableShortEntry{
			{id: 99, name: "foo"},
			{id: 99, name: "bar"},
			{id: 99, name: "bar"},
		}
		assert.False(t, slicekit.ContainsExactly(vs, oth))
	})
}

// namedEntry is a comparable type whose Equal method deliberately
// ignores the id field, so two entries with the same name compare
// equal under ContainsExactly even when their ids differ.
type namedEntry struct {
	id   int
	name string
}

func (e namedEntry) Equal(oth namedEntry) bool {
	return e.name == oth.name
}

// comparableEntry implements predicate.Comparable[T] but NOT
// predicate.Equatable[T]. Compare returns 0 for entries with the
// same name regardless of id, so two values with different ids but
// the same name are "equal" under Compare but not under ==.
//
// This shape lets the test prove that the dispatch reaches for Compare
// specifically: if ContainsExactly/Contains were using == or Equal
// (which this type does not implement), the matching would fail.
type comparableEntry struct {
	id   int
	name string
}

func (e comparableEntry) Compare(oth comparableEntry) int {
	switch {
	case e.name < oth.name:
		return -1
	case e.name > oth.name:
		return 1
	default:
		return 0
	}
}

// comparableShortEntry implements predicate.ComparableShort[T] but
// NOT predicate.Equatable[T]. Cmp returns 0 for entries with the
// same name regardless of id, with the same semantics as
// comparableEntry but using the Cmp short-form signature.
type comparableShortEntry struct {
	id   int
	name string
}

func (e comparableShortEntry) Cmp(oth comparableShortEntry) int {
	switch {
	case e.name < oth.name:
		return -1
	case e.name > oth.name:
		return 1
	default:
		return 0
	}
}

func BenchmarkContainsExactly(b *testing.B) {
	src := rand.NewSource(42)
	rnd := random.New(src)

	const sampling = 1000

	b.Run("string", func(b *testing.B) {
		vs, oth := mkBenchSlices(b, rnd, sampling, rnd.String)
		for b.Loop() {
			slicekit.ContainsExactly(vs, oth)
		}
	})

	b.Run("int", func(b *testing.B) {
		vs, oth := mkBenchSlices(b, rnd, sampling, rnd.Int)
		for b.Loop() {
			slicekit.ContainsExactly(vs, oth)
		}
	})

	b.Run("float", func(b *testing.B) {
		vs, oth := mkBenchSlices(b, rnd, sampling, rnd.Float64)
		for b.Loop() {
			slicekit.ContainsExactly(vs, oth)
		}
	})
}

func mkBenchSlices[T any](b *testing.B, rnd *random.Random, sampling int, mk func() T) ([]T, []T) {
	b.StopTimer()
	defer b.StartTimer()
	vs := random.Slice(sampling, mk)
	oth := slicekit.Clone(vs)
	random.Shuffle(rnd, oth)
	return vs, oth
}
