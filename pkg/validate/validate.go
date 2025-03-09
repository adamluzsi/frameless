package validate

import (
	"reflect"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/option"
)

type Validator interface {
	Validate() error
	// Validate(context.Context) error
}

// Super will validate a value but ignores if the Validator interface is implemented.
// func Super(v any) error {
// 	return Value(v, option.Func[config](func(c *config) {
// 		c.WithoutValidator = true
// 	}))
// }

var interfaceValidator = reflectkit.TypeOf[Validator]()

func Value(v any, opts ...Option) error {
	rv := reflectkit.ToValue(v)
	c := option.Use(opts)

	if rv.Kind() == reflect.Struct {
		return Struct(rv, opts...)
	}

	if err := tryValidatorValidate(rv, c); err != nil {
		return err
	}

	if err := enum.Validate(v); err != nil {
		return ValidationError{Cause: err}
	}

	return nil
}

func Struct(v any, opts ...Option) error {
	rv := reflectkit.ToValue(v)
	c := option.Use(opts)

	if rv.Kind() != reflect.Struct {
		return ImplementationError.F("non struct type type: %s", rv.Type().String())
	}

	if err := tryValidatorValidate(rv, c); err != nil {
		return err
	}

	var (
		T   = rv.Type()
		num = T.NumField()
	)

	for i := 0; i < num; i++ {
		if err := StructField(T.Field(i), rv.Field(i), c); err != nil {
			return err
		}
	}

	return nil
}

func StructField(sf reflect.StructField, field reflect.Value, opts ...Option) error {
	if sf.Type != field.Type() {
		return ImplementationError.F("struct field doesn't belong to the provided field value (%s <=> %s)",
			sf.Type.String(), field.Type().String())
	}

	if err := enum.ValidateStructField(sf, field); err != nil {
		return ValidationError{Cause: err}
	}

	if err := tryValidatorValidate(field, option.Use(opts)); err != nil {
		return err
	}

	return nil
}

type Option option.Option[config]

type config struct {
	WithoutValidator bool
}

func (c config) Configure(t *config) { *t = c }

func tryValidatorValidate(rv reflect.Value, c config) error {
	if c.WithoutValidator {
		return nil
	}
	if !rv.Type().Implements(interfaceValidator) {
		return nil
	}
	outTuble := rv.MethodByName("Validate").Call([]reflect.Value{})
	err, ok := outTuble[0].Interface().(error)
	if ok && err != nil {
		return ValidationError{Cause: err}
	}
	return nil
}
