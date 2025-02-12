package reflectkit

import (
	"reflect"
)

var acceptedConstKind = map[reflect.Kind]struct{}{
	reflect.String:     {},
	reflect.Bool:       {},
	reflect.Int:        {},
	reflect.Int8:       {},
	reflect.Int16:      {},
	reflect.Int32:      {},
	reflect.Int64:      {},
	reflect.Uint:       {},
	reflect.Uint8:      {},
	reflect.Uint16:     {},
	reflect.Uint32:     {},
	reflect.Uint64:     {},
	reflect.Float32:    {},
	reflect.Float64:    {},
	reflect.Complex64:  {},
	reflect.Complex128: {},
}

func IsMutableType(typ reflect.Type) bool {
	if typ == nil {
		return false
	}
	return visitIsMutable(typ)
}

func visitIsMutable(typ reflect.Type) bool {
	if typ.Kind() == reflect.Invalid {
		return false
	}
	if _, ok := acceptedConstKind[typ.Kind()]; ok {
		return false
	}
	if typ.Kind() == reflect.Struct {
		var FieldNum = typ.NumField()
		for i := 0; i < FieldNum; i++ {
			if visitIsMutable(typ.Field(i).Type) {
				return true
			}
		}
		return false
	}
	return true
}
