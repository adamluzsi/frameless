package reflects_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources/specs"
	"github.com/stretchr/testify/require"
)

func ExampleSymbolicName(i interface{}) string {
	return reflects.SymbolicName(i)
}

func TestName(t *testing.T) {
	t.Run("SymbolicName", func(spec *testing.T) {

		subject := reflects.SymbolicName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()

			o := specs.CreatorSpec{}

			require.Equal(t, `specs.CreatorSpec`, ExampleSymbolicName(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			require.Equal(t, `reflects_test.StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, `reflects_test.StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, `reflects_test.StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			require.Equal(t, `reflects_test.StructObject`, subject(&o))
		})

	})

}
