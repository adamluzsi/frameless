package validate_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

type StringTypeThatImplementsValidator string

func (v StringTypeThatImplementsValidator) Validate() error {
	if len(v) == 0 {
		return fmt.Errorf("empty value is not allowed")
	}
	return nil
}

func TestValue_useValidatorInterface(t *testing.T) {
	t.Run("only validator", func(t *testing.T) {
		var v StringTypeThatImplementsValidator = "42"
		assert.NoError(t, validate.Value(v))
	})
	t.Run("combination", func(t *testing.T) {
		t.Run("smoke", func(t *testing.T) {
			v := StructTypeThatImplementsValidator{V: "foo"}
			assert.NoError(t, validate.Value(v))
		})

		t.Run("other fails", func(t *testing.T) {
			v := StructTypeThatImplementsValidator{
				V: "qux", // invalid
			}
			assert.ErrorIs(t, validate.Value(v), enum.ErrInvalid)
		})

		t.Run("Validate fails", func(t *testing.T) {
			expErr := rnd.Error()
			v := StructTypeThatImplementsValidator{
				V:             "foo",
				ValidateError: expErr,
			}
			assert.ErrorIs(t, validate.Value(v), expErr)
		})
	})
	t.Run("rainy", func(t *testing.T) {
		var v StringTypeThatImplementsValidator
		got := validate.Value(v)
		assert.Error(t, got)
		var verr validate.ValidationError
		assert.True(t, errors.As(got, &verr))
		assert.Error(t, verr.Cause)
	})
}

func TestValue_enum(t *testing.T) {
	type FieldType string
	t.Cleanup(enum.Register[FieldType]("foo", "bar", "baz"))

	t.Run("struct", func(t *testing.T) {
		type X struct {
			A FieldType
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Value(X{})
			assert.Error(t, err)

			var verr validate.ValidationError
			assert.True(t, errors.As(err, &verr))
			assert.ErrorIs(t, verr.Cause, enum.ErrInvalid)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Value(X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			assert.NoError(t, err)
		})
	})

	t.Run("value", func(t *testing.T) {
		t.Run("happy", func(t *testing.T) {
			err := validate.Value(random.Pick[FieldType](rnd, "foo", "bar", "baz"))
			assert.NoError(t, err)
		})

		t.Run("rainy", func(t *testing.T) {
			err := validate.Value(FieldType("invalid"))
			assert.Error(t, err)

			var verr validate.ValidationError
			assert.True(t, errors.As(err, &verr))
			assert.ErrorIs(t, verr.Cause, enum.ErrInvalid)
		})
	})
}

func TestStructField_enum(t *testing.T) {
	t.Run("no enum", func(t *testing.T) {
		type X struct {
			A int
		}

		val := reflect.ValueOf(X{A: rnd.Int()})

		sf, field, ok := reflectkit.LookupField(val, "A")
		assert.True(t, ok)

		err := validate.StructField(sf, field)
		assert.NoError(t, err)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		val := reflect.ValueOf(X{})
		sf, field, ok := reflectkit.LookupField(val, "A")
		assert.True(t, ok)

		err := validate.StructField(sf, field)
		assert.Error(t, err)
		assert.ErrorIs(t, enum.ImplementationError, err)
	})
	t.Run("enum defined in the field tag", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		t.Run("zero value", func(t *testing.T) {
			val := reflect.ValueOf(X{})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick(rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(sf, field)
			assert.NoError(t, err)
		})
	})
	t.Run("enum registered to the field type", func(t *testing.T) {
		type FieldType string
		t.Cleanup(enum.Register[FieldType]("foo", "bar", "baz"))

		type X struct {
			A FieldType
		}

		t.Run("zero value", func(t *testing.T) {
			val := reflect.ValueOf(X{})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(sf, field)
			assert.NoError(t, err)
		})
	})
}

func TestStruct_enum(t *testing.T) {
	t.Run("no enum", func(t *testing.T) {
		type X struct {
			A int
		}

		err := validate.Struct(X{A: rnd.Int()})
		assert.NoError(t, err)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		err := validate.Struct(X{})
		assert.Error(t, err)
		assert.ErrorIs(t, enum.ImplementationError, err)
	})
	t.Run("enum defined in the field tag", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Struct(X{})
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(X{A: random.Pick(rnd, "foo", "bar", "baz")})
			assert.NoError(t, err)
		})
	})
	t.Run("enum registered to the field type", func(t *testing.T) {
		type FieldType string
		t.Cleanup(enum.Register[FieldType]("foo", "bar", "baz"))

		type X struct {
			A FieldType
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Struct(X{})
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			assert.NoError(t, err)
		})
	})

	t.Run("when reflect value passed", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Struct(reflect.ValueOf(X{}))
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(reflect.ValueOf(X{A: random.Pick(rnd, "foo", "bar", "baz")}))
			assert.NoError(t, err)
		})
	})
}

type StructTypeThatImplementsValidator struct {
	V string `enum:"foo,bar,baz,"`

	ValidateError error
}

func (v StructTypeThatImplementsValidator) Validate() error {
	return v.ValidateError
}

func TestStruct_useValidatorInterface(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		v := StructTypeThatImplementsValidator{V: "foo"}
		assert.NoError(t, validate.Struct(v))
	})

	t.Run("other fails", func(t *testing.T) {
		v := StructTypeThatImplementsValidator{
			V: "qux", // invalid
		}
		assert.ErrorIs(t, validate.Struct(v), enum.ErrInvalid)
	})

	t.Run("Validate fails", func(t *testing.T) {
		expErr := rnd.Error()
		v := StructTypeThatImplementsValidator{
			V:             "foo",
			ValidateError: expErr,
		}
		assert.ErrorIs(t, validate.Struct(v), expErr)
	})
}

func TestStructField_useValidatorInterface(t *testing.T) {
	type T struct {
		V StructTypeThatImplementsValidator
	}

	t.Run("happy", func(t *testing.T) {
		rStruct := reflect.ValueOf(T{V: StructTypeThatImplementsValidator{V: "foo"}})

		sf, field, ok := reflectkit.LookupField(rStruct, "V")
		assert.True(t, ok)

		assert.NoError(t, validate.StructField(sf, field))
	})

	t.Run("rainy", func(t *testing.T) {
		expErr := rnd.Error()

		rStruct := reflect.ValueOf(T{V: StructTypeThatImplementsValidator{
			V:             "foo",
			ValidateError: expErr,
		}})

		sf, field, ok := reflectkit.LookupField(rStruct, "V")
		assert.True(t, ok)

		err := validate.StructField(sf, field)
		assert.ErrorIs(t, err, expErr)
	})
}
