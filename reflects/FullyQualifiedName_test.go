package reflects_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources/specs"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func ExampleFullyQualifiedName(i interface{}) string {
	return reflects.FullyQualifiedName(i)
}

func TestFullyQualifiedName(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := reflects.FullyQualifiedName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()

			o := specs.CreatorSpec{}

			require.Equal(t, `"github.com/adamluzsi/frameless/resources/specs".CreatorSpec`, ExampleFullyQualifiedName(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			require.Equal(t, `"github.com/adamluzsi/frameless/reflects_test".StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, `"github.com/adamluzsi/frameless/reflects_test".StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, `"github.com/adamluzsi/frameless/reflects_test".StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			require.Equal(t, `"github.com/adamluzsi/frameless/reflects_test".StructObject`, subject(&o))
		})

	})

}

func SpecForPrimitiveNames(spec *testing.T, subject func(entity frameless.Entity) string) {

	spec.Run("when given object is a bool", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "bool", subject(true))
	})

	spec.Run("when given object is a string", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "string", subject(string(42)))
	})

	spec.Run("when given object is a int", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "int", subject(int(42)))
	})

	spec.Run("when given object is a int8", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "int8", subject(int8(42)))
	})

	spec.Run("when given object is a int16", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "int16", subject(int16(42)))
	})

	spec.Run("when given object is a int32", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "int32", subject(int32(42)))
	})

	spec.Run("when given object is a int64", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "int64", subject(int64(42)))
	})

	spec.Run("when given object is a uintptr", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uintptr", subject(uintptr(42)))
	})

	spec.Run("when given object is a uint", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uint", subject(uint(42)))
	})

	spec.Run("when given object is a uint8", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uint8", subject(uint8(42)))
	})

	spec.Run("when given object is a uint16", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uint16", subject(uint16(42)))
	})

	spec.Run("when given object is a uint32", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uint32", subject(uint32(42)))
	})

	spec.Run("when given object is a uint64", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "uint64", subject(uint64(42)))
	})

	spec.Run("when given object is a float32", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "float32", subject(float32(42)))
	})

	spec.Run("when given object is a float64", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "float64", subject(float64(42)))
	})

	spec.Run("when given object is a complex64", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "complex64", subject(complex64(42)))
	})

	spec.Run("when given object is a complex128", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "complex128", subject(complex128(42)))
	})

}
