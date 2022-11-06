package reflects_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/pkg/reflects"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"

	"github.com/adamluzsi/testcase/assert"
)

func ExampleFullyQualifiedName() {
	fmt.Println(reflects.FullyQualifiedName(any(nil)))
}

func TestFullyQualifiedName(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := reflects.FullyQualifiedName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()

			o := crudcontracts.Creator[int, string]{}

			assert.Must(t).Equal(`"github.com/adamluzsi/frameless/ports/crud/contracts".Creator[int,string]`, subject(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			assert.Must(t).Equal(`"github.com/adamluzsi/frameless/pkg/reflects_test".StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`"github.com/adamluzsi/frameless/pkg/reflects_test".StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`"github.com/adamluzsi/frameless/pkg/reflects_test".StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			assert.Must(t).Equal(`"github.com/adamluzsi/frameless/pkg/reflects_test".StructObject`, subject(&o))
		})

	})

}

func SpecForPrimitiveNames(spec *testing.T, subject func(entity interface{}) string) {

	spec.Run("when given object is a bool", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("bool", subject(true))
	})

	spec.Run("when given object is a string", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("string", subject(`42`))
	})

	spec.Run("when given object is a int", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int", subject(int(42)))
	})

	spec.Run("when given object is a int8", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int8", subject(int8(42)))
	})

	spec.Run("when given object is a int16", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int16", subject(int16(42)))
	})

	spec.Run("when given object is a int32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int32", subject(int32(42)))
	})

	spec.Run("when given object is a int64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int64", subject(int64(42)))
	})

	spec.Run("when given object is a uintptr", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uintptr", subject(uintptr(42)))
	})

	spec.Run("when given object is a uint", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint", subject(uint(42)))
	})

	spec.Run("when given object is a uint8", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint8", subject(uint8(42)))
	})

	spec.Run("when given object is a uint16", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint16", subject(uint16(42)))
	})

	spec.Run("when given object is a uint32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint32", subject(uint32(42)))
	})

	spec.Run("when given object is a uint64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint64", subject(uint64(42)))
	})

	spec.Run("when given object is a float32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("float32", subject(float32(42)))
	})

	spec.Run("when given object is a float64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("float64", subject(float64(42)))
	})

	spec.Run("when given object is a complex64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("complex64", subject(complex64(42)))
	})

	spec.Run("when given object is a complex128", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("complex128", subject(complex128(42)))
	})

}
