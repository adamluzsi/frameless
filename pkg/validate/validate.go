package validate

import (
	"reflect"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/reflectkit"
)

func Value(v any) error {
	rv := reflectkit.ToValue(v)

	if rv.Kind() == reflect.Struct {
		return Struct(rv)
	}

	if err := enum.Validate(v); err != nil {
		return err
	}

	return nil
}

func Struct(v any) error {
	rv := reflectkit.ToValue(v)

	if rv.Kind() != reflect.Struct {
		return ImplementationError.F("non struct type type: %s", rv.Type().String())
	}

	var (
		T   = rv.Type()
		num = T.NumField()
	)

	for i := 0; i < num; i++ {
		if err := StructField(T.Field(i), rv.Field(i)); err != nil {
			return err
		}
	}

	return nil
}

func StructField(sf reflect.StructField, field reflect.Value) error {
	if sf.Type != field.Type() {
		return ImplementationError.F("struct field doesn't belong to the provided field value (%s <=> %s)",
			sf.Type.String(), field.Type().String())
	}

	if err := enum.ValidateStructField(sf, field); err != nil {
		return ValidationError{Cause: err}
	}

	return nil
}
