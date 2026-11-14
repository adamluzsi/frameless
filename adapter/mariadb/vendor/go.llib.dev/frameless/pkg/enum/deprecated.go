package enum

import "reflect"

// ReflectValuesOfStructField
//
// Deprecate: use ReflectValues instead
func ReflectValuesOfStructField(field reflect.StructField) ([]reflect.Value, error) {
	if tag, ok := field.Tag.Lookup(enumTagName); ok {
		return parseTag(field.Type, tag)
	}
	return ReflectValues(field.Type), nil
}
