package reflects_test

import (
	"testing"

	"github.com/adamluzsi/frameless/pkg/reflects"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"

	"github.com/adamluzsi/testcase/assert"
)

func ExampleSymbolicName() {
	type T struct{}
	name := reflects.SymbolicName(T{})
	_ = name
}

func TestName(t *testing.T) {
	t.Run("SymbolicName", func(spec *testing.T) {

		subject := reflects.SymbolicName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()
			o := crudcontracts.Creator[int, string]{}
			assert.Must(t).Equal(`crudcontracts.Creator[int,string]`, reflects.SymbolicName(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			assert.Must(t).Equal(`reflects_test.StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`reflects_test.StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`reflects_test.StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			assert.Must(t).Equal(`reflects_test.StructObject`, subject(&o))
		})

	})

}
