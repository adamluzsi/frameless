package pointer_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

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
	assert.Must(t).Equal(&value, vPtr)
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

func ExampleLink() {
	var (
		src string = "Hello, world!"
		dst string
	)
	if err := pointer.Link(src, &dst); err != nil {
		panic(err)
	}
}

func ExampleLink_usingInUnmarshal() {
	var unmarshalFunc = func(data []byte, ptr any) error {
		var out string
		out = string(data) // process data
		return pointer.Link(out, ptr)
	}

	var val string
	if err := unmarshalFunc([]byte("data"), &val); err != nil {
		panic(err)
	}

	// val == "data"
}

func TestLink(t *testing.T) {
	t.Run("happy", testLink)
	t.Run("nil input pointer", testLinkNilInputPointer)
	t.Run("non-pointer input", testLinkNonPointerInput)
	t.Run("incorrect pointer type", testLinkIncorrectPointerType)
}

func testLink(t *testing.T) {
	var (
		src string = rnd.String()
		dst string
	)
	assert.NoError(t, pointer.Link(src, &dst))
	assert.Equal(t, src, dst)
}

func testLinkNilInputPointer(t *testing.T) {
	v := 5
	iptr := (*int)(nil)
	err := pointer.Link[int](v, iptr)
	if err == nil {
		t.Errorf("Expected an error but got none")
	}
}

func testLinkNonPointerInput(t *testing.T) {
	v := 5
	iptr := "string"
	err := pointer.Link[int](v, iptr)
	if err == nil {
		t.Errorf("Expected an error but got none")
	}
}

func testLinkIncorrectPointerType(t *testing.T) {
	v := "string"
	iptr := (*int)(nil)
	err := pointer.Link[string](v, iptr)
	if err == nil {
		t.Errorf("Expected an error but got none")
	}
}
