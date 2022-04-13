package lazyloading_test

import (
	"math/rand"
	"testing"

	"github.com/adamluzsi/frameless/lazyloading"
	"github.com/adamluzsi/testcase/assert"
)

type MyStruct struct {
	lazyLoadedVar lazyloading.Var[int]
}

func (ms *MyStruct) Num() int {
	return ms.lazyLoadedVar.Do(func() int {
		return rand.Int()
	})
}

func ExampleVar() {
	ms := &MyStruct{}

	ms.Num() // uses lazy loading under the hood
}

func TestMyStruct_Value(t *testing.T) {
	ms1 := &MyStruct{}
	assert.Must(t).Equal(ms1.Num(), ms1.Num())

	ms2 := &MyStruct{}
	assert.Must(t).NotEqual(ms1.Num(), ms2.Num())
}
