# Package merge

Merge package helps you to merge various types like slice, map and error. 

```go
package main

import (
	"errors"
	"go.llib.dev/frameless/pkg/merge"
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

func ExampleError() {
	var (
		err1 error = errors.New("first error")
		err2 error = errors.New("second error")
		err3 error = nil
	)

	err := merge.Error(err1, err2, err3)
	errors.Is(err, err1) // true
	errors.Is(err, err2) // true
	errors.Is(err, err3) // true
}
```
