package reflectkit

import (
	"iter"
	"reflect"

	"go.llib.dev/frameless/internal/interr"
)

func IterStructFields(rStruct reflect.Value) iter.Seq2[reflect.StructField, reflect.Value] {
	if rStruct.Kind() != reflect.Struct {
		panic(interr.ImplementationError.F("expected %s to be a struct type", rStruct.Type().String()))
	}
	return iter.Seq2[reflect.StructField, reflect.Value](func(yield func(reflect.StructField, reflect.Value) bool) {
		var (
			typ = rStruct.Type()
			num = typ.NumField()
		)
		for i := 0; i < num; i++ {
			if !yield(typ.Field(i), rStruct.Field(i)) {
				break
			}
		}
	})
}

func IterMap(rMap reflect.Value) iter.Seq2[reflect.Value, reflect.Value] {
	if rMap.Kind() != reflect.Map {
		panic(interr.ImplementationError.F("expected %s to be a map type", rMap.Type().String()))
	}
	return iter.Seq2[reflect.Value, reflect.Value](func(yield func(reflect.Value, reflect.Value) bool) {
		i := rMap.MapRange()
		for i.Next() {
			if !yield(i.Key(), i.Value()) {
				break
			}
		}
	})
}

func IterSlice(rSlice reflect.Value) iter.Seq2[int, reflect.Value] {
	if rSlice.Kind() != reflect.Slice {
		panic(interr.ImplementationError.F("expected %s to be a slice type", rSlice.Type().String()))
	}
	return iter.Seq2[int, reflect.Value](func(yield func(int, reflect.Value) bool) {
		var length = rSlice.Len()
		for i := 0; i < length; i++ {
			if !yield(i, rSlice.Index(i)) {
				break
			}
		}
	})
}
