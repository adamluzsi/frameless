package memory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase/assert"
)

func TestMakeID(t *testing.T) {
	// int
	AssertIDValue[int](t, memory.MakeID[int])
	AssertIDValue[int8](t, memory.MakeID[int8])
	AssertIDValue[int16](t, memory.MakeID[int16])
	AssertIDValue[int32](t, memory.MakeID[int32])
	AssertIDValue[int64](t, memory.MakeID[int64])
	type MyInt int
	AssertIDValue[MyInt](t, memory.MakeID[MyInt])
	// string
	AssertIDValue[string](t, memory.MakeID[string])
	type MyString string
	AssertIDValue[MyString](t, memory.MakeID[MyString])
}

func AssertIDValue[ID any](tb testing.TB, MakeFunc func(context.Context) (ID, error)) {
	tb.Helper()

	id, err := MakeFunc(context.Background())
	assert.NoError(tb, err)
	assert.NotEmpty(tb, id)

	othID, err := MakeFunc(context.Background())
	assert.NoError(tb, err)
	assert.NotEqual(tb, id, othID)
}
