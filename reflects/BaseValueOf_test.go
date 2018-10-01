package reflects_test

import (
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
	"testing"
)


func TestBaseValueOf(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := func(obj interface{}) string {
			return reflects.BaseValueOf(obj).Type().Name()
		}

		SpecForPrimitiveNames(spec, subject)

		cases := make(map[interface{}]reflect.Value)

		cases[StructObject{}] = reflect.ValueOf(StructObject{})
		cases[&StructObject{}] = reflect.ValueOf(StructObject{})
		o := &StructObject{}
		cases[&o] = reflect.ValueOf(StructObject{})

	})
}
