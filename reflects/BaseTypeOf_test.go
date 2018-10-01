package reflects_test

import (
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
	"testing"
)


func TestBaseTypeOf(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := func(obj interface{}) string {
			return reflects.BaseTypeOf(obj).Name()
		}

		SpecForPrimitiveNames(spec, subject)

		cases := make(map[interface{}]reflect.Type)

		cases[StructObject{}] = reflect.TypeOf(StructObject{})
		cases[&StructObject{}] = reflect.TypeOf(StructObject{})
		o := &StructObject{}
		cases[&o] = reflect.TypeOf(StructObject{})

	})
}
