package validate_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase"
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
		var verr validate.Error
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

			var verr validate.Error
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

			var verr validate.Error
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

func Example_rangeInt() {
	type T struct {
		V int `range:"0..100"`
	}

	validate.Value(T{V: 42})  // no error
	validate.Value(T{V: -1})  // validate.Error
	validate.Value(T{V: 101}) // validate.Error
}

func Example_rangeIntMulti() {
	type T struct {
		Num1 int `range:"0..100"`
		Num2 int `range:"0..25,30..50"`
	}

	_ = validate.Value(T{})
}

func Test_range(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("int", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V int `range:"0..100"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V int `range:"0..100,576..1024"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(576, 1024)}))
		})

		s.Test("SPACE", func(t *testcase.T) {
			type T struct {
				V int `range:" 0 .. 100 , 576 .. 1024 "`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(0, 100)}))
			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(576, 1024)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V int `range:"..100"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(-100, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V int `range:"0.."`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("minus-min", func(t *testcase.T) {
			type T struct {
				V int `range:"-1..10"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(-1, 10)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: -1 - t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(T{V: 10 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("minus-max", func(t *testcase.T) {
			type T struct {
				V int `range:"-100..-10"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.IntBetween(-100, -10)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: -100 - t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(T{V: -10 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("swapped-range", func(t *testcase.T) {
			type T struct {
				V int `range:"-1..-10"`
			}

			assert.NoError(t, validate.Value(T{V: -5}))
			assert.Error(t, validate.Value(T{V: -11}))
		})
	})

	s.Context("string", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V string `range:"a..c"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `range:"a..c,e..g"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abcefg")}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: t.Random.StringNC(1, "dklm")}), &got))
		})

		s.Test("with-length", func(t *testcase.T) {
			type T struct {
				V string `range:"a..ccc"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(3, "abc")}))
			assert.NoError(t, validate.Value(T{V: "ccc"}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: t.Random.StringNC(1, "defg")}), &got))
			assert.True(t, errors.As(validate.Value(T{V: "ccd"}), &got))
		})
	})

	s.Test("float", func(t *testcase.T) {
		type T struct {
			V float64 `range:"0..100"`
		}

		assert.NoError(t, validate.Value(T{V: float64(t.Random.IntBetween(0, 100))}))
		assert.Error(t, validate.Value(T{V: float64(t.Random.IntBetween(101, 105))}))
		assert.Error(t, validate.Value(T{V: float64(t.Random.IntBetween(-10, -1))}))

		var got validate.Error
		assert.True(t, errors.As(validate.Value(T{V: float64(t.Random.IntBetween(-10, -1))}), &got))
	})

	s.Describe("as character-range", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V string `range:"a-c"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))

			assert.Error(t, validate.Value(T{V: "d"}))
			assert.Error(t, validate.Value(T{V: "dd"}))
			assert.Error(t, validate.Value(T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("subtype", func(t *testcase.T) {
			type STR string
			type T struct {
				V STR `range:"a-c"`
			}

			assert.NoError(t, validate.Value(T{V: STR(t.Random.StringNC(1, "abc"))}))
			assert.NoError(t, validate.Value(T{V: STR(t.Random.StringNC(t.Random.IntBetween(3, 7), "abc"))}))

			assert.Error(t, validate.Value(T{V: "d"}))
			assert.Error(t, validate.Value(T{V: "dd"}))
			assert.Error(t, validate.Value(T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: STR(t.Random.StringNC(1, "defg"))}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `range:"a-c,e-g"`
			}

			length := t.Random.IntBetween(3, 7)

			t.Log("first range")
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(length, "abc")}))

			t.Log("second range")
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "efg")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(length, "efg")}))

			t.Log("mixed range")
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abcefg")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(length, "abcefg")}))

			// assert.Error(t, validate.Value(T{V: "d"}))
			// assert.Error(t, validate.Value(T{V: "dd"}))
			// assert.Error(t, validate.Value(T{V: "dddd"}))
			// assert.Error(t, validate.Value(T{V: "avb" + t.Random.StringNC(1, "ijklmnopqrstuvwxyz")}))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V string `range:"-c"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))
			assert.Error(t, validate.Value(T{V: t.Random.StringNC(1, string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V string `range:"d-"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(1, 7), string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
			assert.Error(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
		})

		s.Describe("rune", func(s *testcase.Spec) {
			s.Test("single range range", func(t *testcase.T) {
				type T struct {
					V rune `range:"a-c"`
				}

				assert.NoError(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "abc"))[0]}))
				assert.Error(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
			})
			s.Test("multi range range", func(t *testcase.T) {
				type T struct {
					V rune `range:"a-c,e-g"`
				}

				assert.NoError(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "abcefg"))[0]}))
				assert.Error(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
			})
		})

		s.Test("invalid character range tag value", func(t *testcase.T) {
			type NonSingleCharMin struct {
				V string `range:"aa-c"`
			}

			assert.Panic(t, func() { validate.Value(NonSingleCharMin{}) })

			type NonSingleCharMax struct {
				V string `range:"a-cc"`
			}

			assert.Panic(t, func() { validate.Value(NonSingleCharMax{}) })
		})

		s.Test("mixed with classic string ranges", func(t *testcase.T) {
			type T struct {
				V string `range:"d-l,aaaa..cccc"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(1, 99), "defghijkl")}), "expected that char range match the given value")
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(4, "abc")}), "expected that string range includes the given value")

			assert.Error(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(1, 4), "xyz")}),
				"expected that it is outside of the char range and also not covered by the string range")
		})
	})
}

func Test_char(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe("string", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V string `char:"a-c"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))

			assert.Error(t, validate.Value(T{V: "d"}))
			assert.Error(t, validate.Value(T{V: "dd"}))
			assert.Error(t, validate.Value(T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("subtype", func(t *testcase.T) {
			type STR string
			type T struct {
				V STR `char:"a-c"`
			}

			assert.NoError(t, validate.Value(T{V: STR(t.Random.StringNC(1, "abc"))}))
			assert.NoError(t, validate.Value(T{V: STR(t.Random.StringNC(t.Random.IntBetween(3, 7), "abc"))}))

			assert.Error(t, validate.Value(T{V: "d"}))
			assert.Error(t, validate.Value(T{V: "dd"}))
			assert.Error(t, validate.Value(T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(T{V: STR(t.Random.StringNC(1, "defg"))}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `char:"a-c,e-g"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abcefg")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abcefg")}))

			assert.Error(t, validate.Value(T{V: "d"}))
			assert.Error(t, validate.Value(T{V: "dd"}))
			assert.Error(t, validate.Value(T{V: "dddd"}))
			assert.Error(t, validate.Value(T{V: "avb" + t.Random.StringNC(1, "ijklmnopqrstuvwxyz")}))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V string `char:"-c"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))
			assert.Error(t, validate.Value(T{V: t.Random.StringNC(1, string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V string `char:"d-"`
			}

			assert.NoError(t, validate.Value(T{V: t.Random.StringNC(t.Random.IntBetween(1, 7), string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
			assert.Error(t, validate.Value(T{V: t.Random.StringNC(1, "abc")}))
		})
	})

	s.Describe("rune", func(s *testcase.Spec) {
		s.Test("single char range", func(t *testcase.T) {
			type T struct {
				V rune `char:"a-c"`
			}

			assert.NoError(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "abc"))[0]}))
			assert.Error(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
		})
		s.Test("multi char range", func(t *testcase.T) {
			type T struct {
				V rune `char:"a-c,e-g"`
			}

			assert.NoError(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "abcefg"))[0]}))
			assert.Error(t, validate.Value(T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
		})
	})

	s.Test("invalid char tag", func(t *testcase.T) {
		type NonSingleCharMin struct {
			V string `char:"aa-c"`
		}

		assert.Panic(t, func() { validate.Value(NonSingleCharMin{}) })

		type NonSingleCharMax struct {
			V string `char:"a-cc"`
		}

		assert.Panic(t, func() { validate.Value(NonSingleCharMax{}) })
	})
}
