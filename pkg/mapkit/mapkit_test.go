package mapkit_test

import (
	"fmt"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/testcase/assert"
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
