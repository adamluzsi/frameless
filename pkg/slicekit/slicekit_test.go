package slicekit_test

import (
	"fmt"
	tsk "go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/assert"
	"strconv"
	"strings"
	"testing"
)

func ExampleMust() {
	var x = []int{1, 2, 3}
	x = tsk.Must(tsk.Map[int](x, func(v int) int {
		return v * 2
	}))

	v := tsk.Must(tsk.Reduce[int](x, 42, func(output int, current int) int {
		return output + current
	}))

	fmt.Println("result:", v)
}

func TestMust(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got := tsk.Must(tsk.Map[int](x, strconv.Atoi))
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		pv := assert.Panic(t, func() {
			tsk.Must(tsk.Map[int](x, strconv.Atoi))
		})
		err, ok := pv.(error)
		assert.True(t, ok)
		assert.Error(t, err)
	})
}

func ExampleMap() {
	var x = []string{"a", "b", "c"}
	_ = tsk.Must(tsk.Map[string](x, strings.ToUpper)) // []string{"A", "B", "C"}

	var ns = []string{"1", "2", "3"}
	_, err := tsk.Map[int](ns, strconv.Atoi) // []int{1, 2, 3}
	if err != nil {
		panic(err)
	}
}

func TestMap(t *testing.T) {
	t.Run("happy - no error", func(t *testing.T) {
		var x = []string{"a", "b", "c"}
		got, err := tsk.Map[string](x, strings.ToUpper)
		assert.NoError(t, err)
		assert.Equal(t, []string{"A", "B", "C"}, got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := tsk.Map[int](x, strconv.Atoi)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
	})
	t.Run("rainy", func(t *testing.T) {
		var x = []string{"1", "B", "3"}
		_, err := tsk.Map[int](x, strconv.Atoi)
		assert.Error(t, err)
	})
}

func ExampleReduce() {
	var x = []string{"a", "b", "c"}
	got, err := tsk.Reduce[string](x, "|", func(o string, i string) string {
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
		got, err := tsk.Reduce[string](x, "|", func(o string, i string) string {
			return o + i
		})
		assert.NoError(t, err)
		assert.Equal(t, "|abc", got)
	})
	t.Run("happy", func(t *testing.T) {
		var x = []string{"1", "2", "3"}
		got, err := tsk.Reduce[int](x, 42, func(o int, i string) (int, error) {
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
		_, err := tsk.Reduce[int](x, 0, func(o int, i string) (int, error) {
			n, err := strconv.Atoi(i)
			if err != nil {
				return o, err
			}
			return o + n, nil
		})
		assert.Error(t, err)
	})
}
