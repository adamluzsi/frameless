package pointer_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type ExampleStruct struct {
	StrPtrField *string
	IntPtrField *int
}

func ExampleOf() {
	_ = ExampleStruct{
		StrPtrField: pointer.Of("42"),
		IntPtrField: pointer.Of(42),
	}
}

func TestOf(tt *testing.T) {
	t := testcase.ToT(tt)
	var value = t.Random.String()
	vPtr := pointer.Of(value)
	t.Must.Equal(&value, vPtr)
}

func ExampleDeref() {
	var es ExampleStruct
	_ = pointer.Deref(es.StrPtrField)
	_ = pointer.Deref(es.IntPtrField)
}

func TestDeref(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, zero value returned", func(t *testing.T) {
		var str *string
		got := pointer.Deref(str)
		assert.Equal[string](t, "", got)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		got := pointer.Deref(&expected)
		assert.Equal[string](t, expected, got)
	})
}
