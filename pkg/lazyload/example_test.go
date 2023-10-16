package lazyload_test

import (
	"fmt"
	"math/rand"
	"testing"

	"go.llib.dev/frameless/pkg/lazyload"
	"github.com/adamluzsi/testcase/assert"
)

type MyStruct struct {
	lazyLoadedVariable lazyload.Var[int]
}

func (ms *MyStruct) Num() int {
	return ms.lazyLoadedVariable.Get(func() int {
		return rand.Int()
	})
}

func ExampleVar_asStructField() {
	ms := &MyStruct{}

	ms.Num() // uses lazy loading under the hood
}

func TestVar_asStructField(t *testing.T) {
	ms1 := &MyStruct{}
	assert.Must(t).Equal(ms1.Num(), ms1.Num())

	ms2 := &MyStruct{}
	assert.Must(t).NotEqual(ms1.Num(), ms2.Num())
}

func ExampleVar_Get() {
	myInt := lazyload.Var[int]{
		Init: func() int {
			return 42
		},
	}
	_ = myInt.Get() // get 42
}

func ExampleVar_Get_withInitBlock() {
	// This use-case is ideal for initializing lazyload.Var s in struct fields,
	// where defining Init at high level is not possible.
	type MyStruct struct {
		myInt lazyload.Var[int]
	}

	r := &MyStruct{}
	// func (r *MyType) getValue() int {
	getValue := func() int {
		return r.myInt.Get(func() int {
			return 42
		})
	}

	_ = getValue() // 42
}

func ExampleMake() {
	value := lazyload.Make(func() int {
		// my expensive calculation's result
		return 42
	})

	// one eternity later
	fmt.Println(value())
	fmt.Println(value())
}
