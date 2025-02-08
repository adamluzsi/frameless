package validate_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

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
