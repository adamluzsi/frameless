package reflects_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func ExampleName(i interface{}) string {
	return reflects.Name(i)
}

func TestName(suite *testing.T) {
	suite.Run("Name", func(spec *testing.T) {

		spec.Run("when given object is a primitive", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "string", ExampleName("hello"))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			require.Equal(t, "StructObject", ExampleName(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "StructObject", ExampleName(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "StructObject", ExampleName(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			require.Equal(t, "StructObject", ExampleName(&o))
		})

	})
}
