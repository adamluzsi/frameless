package reflects_test

import (
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
	"testing"
)


func TestName(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := func(obj interface{}) string {
			return reflects.BaseTypeOf(obj).Name()
		}

		SpecForPrimitiveNames(spec, subject)

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			require.Equal(t, "StructObject", subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "StructObject", subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "StructObject", subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			require.Equal(t, "StructObject", subject(&o))
		})

	})
}
