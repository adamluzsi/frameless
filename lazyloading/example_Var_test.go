package lazyloading_test

import (
	"math/rand"
	"testing"

	"github.com/adamluzsi/frameless/lazyloading"
	"github.com/stretchr/testify/require"
)

type MyStruct struct {
	lazyLoadedVar lazyloading.Var
}

func (ms *MyStruct) Num() int {
	return ms.lazyLoadedVar.Do(func() interface{} {
		return rand.Int()
	}).(int)
}

func ExampleVar() {
	ms := &MyStruct{}

	ms.Num() // uses lazy loading under the hood
}

func TestMyStruct_Value(t *testing.T) {
	ms1 := &MyStruct{}
	require.Equal(t, ms1.Num(), ms1.Num())

	ms2 := &MyStruct{}
	require.NotEqual(t, ms1.Num(), ms2.Num())
}
