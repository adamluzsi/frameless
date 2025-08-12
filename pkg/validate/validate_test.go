package validate_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

type StringValidatorStub string

func (v StringValidatorStub) Validate(context.Context) error {
	if v == "invalid" {
		return fmt.Errorf("'invalid' is an invalid value")
	}
	return nil
}

type StructValidatorStub struct {
	ValidateError error
}

func (v StructValidatorStub) Validate(context.Context) error {
	return v.ValidateError
}

func TestValue_useValidatorInterface(t *testing.T) {
	t.Run("only validator", func(t *testing.T) {
		var v StringValidatorStub = "42"
		assert.NoError(t, validate.Value(context.Background(), v))
	})
	t.Run("combination", func(t *testing.T) {
		t.Run("smoke", func(t *testing.T) {
			v := StructTypeThatImplementsValidator{V: "foo"}
			assert.NoError(t, validate.Value(context.Background(), v))
		})

		t.Run("other fails", func(t *testing.T) {
			v := StructTypeThatImplementsValidator{
				V: "qux", // invalid
			}
			assert.ErrorIs(t, validate.Value(context.Background(), v), enum.ErrInvalid)
		})

		t.Run("Validate fails", func(t *testing.T) {
			expErr := rnd.Error()
			v := StructTypeThatImplementsValidator{
				V:             "foo",
				ValidateError: expErr,
			}
			assert.ErrorIs(t, validate.Value(context.Background(), v), expErr)
		})
	})
	t.Run("rainy", func(t *testing.T) {
		var v StringValidatorStub = "invalid"
		got := validate.Value(context.Background(), v)
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
			err := validate.Value(context.Background(), X{})
			assert.Error(t, err)

			var verr validate.Error
			assert.True(t, errors.As(err, &verr))
			assert.ErrorIs(t, verr.Cause, enum.ErrInvalid)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Value(context.Background(), X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			assert.NoError(t, err)
		})
	})

	t.Run("value", func(t *testing.T) {
		t.Run("happy", func(t *testing.T) {
			err := validate.Value(context.Background(), random.Pick[FieldType](rnd, "foo", "bar", "baz"))
			assert.NoError(t, err)
		})

		t.Run("rainy", func(t *testing.T) {
			err := validate.Value(context.Background(), FieldType("invalid"))
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

		err := validate.StructField(context.Background(), sf, field)
		assert.NoError(t, err)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		val := reflect.ValueOf(X{})
		sf, field, ok := reflectkit.LookupField(val, "A")
		assert.True(t, ok)

		err := validate.StructField(context.Background(), sf, field)
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

			err := validate.StructField(context.Background(), sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick(rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(context.Background(), sf, field)
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

			err := validate.StructField(context.Background(), sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := validate.StructField(context.Background(), sf, field)
			assert.NoError(t, err)
		})
	})

	t.Run("struct field enum values are validated", func(t *testing.T) {
		type E string
		defer enum.Register[E]("foo", "bar", "baz")()

		type T struct {
			V E
		}

		okVal := random.Pick(rnd, enum.Values[E]()...)
		sf, sv := must.OK2(reflectkit.LookupField(reflect.ValueOf(T{V: okVal}), "V"))

		assert.NoError(t, validate.StructField(t.Context(), sf, sv))

		sf2, sv2 := must.OK2(reflectkit.LookupField(reflect.ValueOf(T{V: "hello"}), "V"))
		err := validate.StructField(t.Context(), sf2, sv2)

		assert.ErrorIs(t, err, enum.ErrInvalid)
		var verr validate.Error
		assert.True(t, errors.As(err, &verr))
		assert.ErrorIs(t, verr.Cause, enum.ErrInvalid)
	})
}

func TestStruct_enum(t *testing.T) {
	t.Run("no enum", func(t *testing.T) {
		type X struct {
			A int
		}

		err := validate.Struct(t.Context(), X{A: rnd.Int()})
		assert.NoError(t, err)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		err := validate.Struct(t.Context(), X{})
		assert.Error(t, err)
		assert.ErrorIs(t, enum.ImplementationError, err)
	})
	t.Run("enum defined in the field tag", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Struct(t.Context(), X{})
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(t.Context(), X{A: random.Pick(rnd, "foo", "bar", "baz")})
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
			err := validate.Struct(t.Context(), X{})
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(t.Context(), X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			assert.NoError(t, err)
		})
	})

	t.Run("when reflect value passed", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		t.Run("zero value", func(t *testing.T) {
			err := validate.Struct(t.Context(), reflect.ValueOf(X{}))
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			err := validate.Struct(t.Context(), reflect.ValueOf(X{A: random.Pick(rnd, "foo", "bar", "baz")}))
			assert.NoError(t, err)
		})
	})
}

type StructTypeThatImplementsValidator struct {
	V string `enum:"foo,bar,baz,"`

	ValidateError error
}

func (v StructTypeThatImplementsValidator) Validate(context.Context) error {
	return v.ValidateError
}

func TestStruct_useValidatorInterface(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		v := StructTypeThatImplementsValidator{V: "foo"}
		assert.NoError(t, validate.Struct(t.Context(), v))
	})

	t.Run("other fails", func(t *testing.T) {
		v := StructTypeThatImplementsValidator{
			V: "qux", // invalid
		}
		assert.ErrorIs(t, validate.Struct(t.Context(), v), enum.ErrInvalid)
	})

	t.Run("Validate fails", func(t *testing.T) {
		expErr := rnd.Error()
		v := StructTypeThatImplementsValidator{
			V:             "foo",
			ValidateError: expErr,
		}
		assert.ErrorIs(t, validate.Struct(t.Context(), v), expErr)
	})
}

func TestStructField_struct(t *testing.T) {
	type T struct {
		V StructValidatorStub
	}

	var v T
	sf1, sv1 := must.OK2(reflectkit.LookupField(reflect.ValueOf(v), "V"))
	assert.NoError(t, validate.StructField(t.Context(), sf1, sv1))

	expErr := rnd.Error()
	v.V.ValidateError = expErr

	sf2, sv2 := must.OK2(reflectkit.LookupField(reflect.ValueOf(v), "V"))
	assert.ErrorIs(t, expErr, validate.StructField(t.Context(), sf2, sv2))
}

func TestSTructField_tagTakesPriorityOverType(t *testing.T) {
	s := testcase.NewSpec(t)

	t.Log("tag validation takes priority over actual enum value")
	type E string
	defer enum.Register[E]("foo", "bar", "baz")()

	type T struct {
		V E `enum:"hello world"`
	}

	s.Test("happy", func(t *testcase.T) {
		var v = T{V: E(random.Pick(t.Random, "hello", "world"))}
		assert.NoError(t, enum.ValidateStruct(v))
		assert.NoError(t, validate.Value(context.Background(), v))
		assert.NoError(t, validate.Struct(t.Context(), v))
		assert.NoError(t, validate.StructField(toStructField(t, v, "V")))
	})

	s.Test("rainy", func(t *testcase.T) {
		var v = T{V: E(random.Pick(t.Random, "qux", "quux", "corge", "grault", "garply"))}
		assert.ErrorIs(t, enum.ErrInvalid, enum.ValidateStruct(v))
		assert.ErrorIs(t, enum.ErrInvalid, validate.Value(context.Background(), v))
		assert.ErrorIs(t, enum.ErrInvalid, validate.Struct(t.Context(), v))
		assert.ErrorIs(t, enum.ErrInvalid, validate.StructField(toStructField(t, v, "V")))
	})
}

func toStructField(tb testing.TB, Struct any, fieldName string) (context.Context, reflect.StructField, reflect.Value) {
	rStruct := reflectkit.ToValue(Struct)
	field, value, ok := reflectkit.LookupField(rStruct, fieldName)
	assert.True(tb, ok, assert.MessageF("expected that %s has a %s field", rStruct.Type().String(), fieldName))
	return tb.Context(), field, value
}

func TestStructField_useValidatorInterface(t *testing.T) {
	type T struct {
		V StructTypeThatImplementsValidator
	}

	t.Run("happy", func(t *testing.T) {
		rStruct := reflect.ValueOf(T{V: StructTypeThatImplementsValidator{V: "foo"}})

		sf, sv, ok := reflectkit.LookupField(rStruct, "V")
		assert.True(t, ok)

		assert.NoError(t, validate.StructField(t.Context(), sf, sv))
	})

	t.Run("rainy", func(t *testing.T) {
		expErr := rnd.Error()

		rStruct := reflect.ValueOf(T{V: StructTypeThatImplementsValidator{
			V:             "foo",
			ValidateError: expErr,
		}})

		sf, sv, ok := reflectkit.LookupField(rStruct, "V")
		assert.True(t, ok)

		err := validate.StructField(t.Context(), sf, sv)
		assert.ErrorIs(t, err, expErr)
	})
}

func Example_rangeInt() {
	type T struct {
		V int `range:"0..100"`
	}

	ctx := context.Background()
	validate.Value(ctx, T{V: 42})  // no error
	validate.Value(ctx, T{V: -1})  // validate.Error
	validate.Value(ctx, T{V: 101}) // validate.Error
}

func Example_rangeIntMulti() {
	type T struct {
		Num1 int `range:"0..100"`
		Num2 int `range:"0..25,30..50"`
	}

	ctx := context.Background()
	_ = validate.Value(ctx, T{})
}

func Test_range(t *testing.T) {
	s := testcase.NewSpec(t)

	var Struct = let.Var[any](s, nil)

	s.Context("string", func(s *testcase.Spec) {
		s.Context("lexical-range", func(s *testcase.Spec) {
			type T struct {
				V string `range:"aaa..ccc"`
			}

			s.When("string is within range", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.StringNC(3, "abc")}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("string is out of range", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.StringNC(3, "def")}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("character-range", func(s *testcase.Spec) {
			type T struct {
				V string `range:"a-z"`
			}

			s.When("string is within range", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.StringNC(3, strings.ToLower(random.CharsetAlpha()))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("string is out of range", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.StringNC(3, strings.ToUpper(random.CharsetAlpha()))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})
	})

	s.Context("int", func(s *testcase.Spec) {
		type T struct {
			V int `range:"5..100"`
		}

		s.When("number is within range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.IntBetween(5, 100)}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is not within range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: random.Pick(t.Random,
					t.Random.IntBetween(-100, 4),
					t.Random.IntBetween(101, 200))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("uint", func(s *testcase.Spec) {
		type T struct {
			V uint `range:"5..10"`
		}

		s.When("number is within range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: uint(t.Random.IntBetween(5, 10))}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is not within range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: random.Pick(t.Random,
					uint(t.Random.IntBetween(0, 4)),
					uint(t.Random.IntBetween(11, 100)))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("float", func(s *testcase.Spec) {
		type T struct {
			V float64 `range:"4.5..10"`
		}

		s.When("float is within range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.FloatBetween(4.5, 10)}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("float is out of range", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: random.Pick(t.Random,
					t.Random.FloatBetween(-100, 4.4),
					t.Random.FloatBetween(10.1, 25),
				)}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("int", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V int `range:"0..100"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V int `range:"0..100,576..1024"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(576, 1024)}))
		})

		s.Test("SPACE", func(t *testcase.T) {
			type T struct {
				V int `range:" 0 .. 100 , 576 .. 1024 "`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(0, 100)}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(576, 1024)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V int `range:"..100"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(-100, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 100 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V int `range:"0.."`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(0, 100)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 0 - t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("minus-min", func(t *testcase.T) {
			type T struct {
				V int `range:"-1..10"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(-1, 10)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: -1 - t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: 10 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("minus-max", func(t *testcase.T) {
			type T struct {
				V int `range:"-100..-10"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.IntBetween(-100, -10)}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: -100 - t.Random.IntBetween(1, 100)}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: -10 + t.Random.IntBetween(1, 100)}), &got))
		})

		s.Test("swapped-range", func(t *testcase.T) {
			type T struct {
				V int `range:"-1..-10"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: -5}))
			assert.Error(t, validate.Value(t.Context(), T{V: -11}))
		})
	})

	s.Context("string", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V string `range:"a..c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `range:"a..c,e..g"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abcefg")}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: t.Random.StringNC(1, "dklm")}), &got))
		})

		s.Test("with-length", func(t *testcase.T) {
			type T struct {
				V string `range:"a..ccc"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(3, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: "ccc"}))

			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: t.Random.StringNC(1, "defg")}), &got))
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: "ccd"}), &got))
		})
	})

	s.Test("float", func(t *testcase.T) {
		type T struct {
			V float64 `range:"0..100"`
		}

		assert.NoError(t, validate.Value(t.Context(), T{V: float64(t.Random.IntBetween(0, 100))}))
		assert.Error(t, validate.Value(t.Context(), T{V: float64(t.Random.IntBetween(101, 105))}))
		assert.Error(t, validate.Value(t.Context(), T{V: float64(t.Random.IntBetween(-10, -1))}))

		var got validate.Error
		assert.True(t, errors.As(validate.Value(t.Context(), T{V: float64(t.Random.IntBetween(-10, -1))}), &got))
	})

	s.Describe("as character-range", func(s *testcase.Spec) {
		s.Test("smoke", func(t *testcase.T) {
			type T struct {
				V string `range:"a-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))

			assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("subtype", func(t *testcase.T) {
			type STR string
			type T struct {
				V STR `range:"a-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: STR(t.Random.StringNC(1, "abc"))}))
			assert.NoError(t, validate.Value(t.Context(), T{V: STR(t.Random.StringNC(t.Random.IntBetween(3, 7), "abc"))}))

			assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: STR(t.Random.StringNC(1, "defg"))}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `range:"a-c,e-g"`
			}

			length := t.Random.IntBetween(3, 7)

			t.Log("first range")
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(length, "abc")}))

			t.Log("second range")
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "efg")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(length, "efg")}))

			t.Log("mixed range")
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abcefg")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(length, "abcefg")}))

			// assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			// assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			// assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			// assert.Error(t, validate.Value(t.Context(), T{V: "avb" + t.Random.StringNC(1, "ijklmnopqrstuvwxyz")}))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V string `range:"-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))
			assert.Error(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V string `range:"d-"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(1, 7), string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
			assert.Error(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
		})

		s.Describe("rune", func(s *testcase.Spec) {
			s.Test("single range range", func(t *testcase.T) {
				type T struct {
					V rune `range:"a-c"`
				}

				assert.NoError(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "abc"))[0]}))
				assert.Error(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
			})
			s.Test("multi range range", func(t *testcase.T) {
				type T struct {
					V rune `range:"a-c,e-g"`
				}

				assert.NoError(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "abcefg"))[0]}))
				assert.Error(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
			})
		})

		s.Test("invalid character range tag value", func(t *testcase.T) {
			type NonSingleCharMin struct {
				V string `range:"aa-c"`
			}

			assert.Panic(t, func() { validate.Value(t.Context(), NonSingleCharMin{}) })

			type NonSingleCharMax struct {
				V string `range:"a-cc"`
			}

			assert.Panic(t, func() { validate.Value(t.Context(), NonSingleCharMax{}) })
		})

		s.Test("mixed with classic string ranges", func(t *testcase.T) {
			type T struct {
				V string `range:"d-l,aaaa..cccc"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(1, 99), "defghijkl")}), "expected that char range match the given value")
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(4, "abc")}), "expected that string range includes the given value")

			assert.Error(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(1, 4), "xyz")}),
				"expected that it is outside of the char range and also not covered by the string range")
		})
	})

	s.Describe("alternative comparison format style", func(s *testcase.Spec) {
		s.Context("= (equals sign)", func(s *testcase.Spec) {
			type T struct {
				V int `range:"=42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: 42}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Unique(t.Random.Int, 42)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("!= (not equal to)", func(s *testcase.Spec) {
			type T struct {
				V int `range:"!=42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Unique(t.Random.Int, 42)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: 42}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("> (greater than)", func(s *testcase.Spec) {
			type T struct {
				V int `range:">42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(43, 100)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(-42, 42)}
				})

				ThenErrorIsExpected(s, Struct)
			})

			s.Context("Yoda conditions x<", func(s *testcase.Spec) {
				type T struct {
					V int `range:"42<"`
				}

				s.Context("within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(43, 100)}
					})

					ThenNoErrorIsExpected(s, Struct)
				})

				s.Context("not-within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(-42, 42)}
					})

					ThenErrorIsExpected(s, Struct)
				})

			})
		})

		s.Context("< (less than)", func(s *testcase.Spec) {
			type T struct {
				V int `range:"<42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(-42, 41)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(42, 100)}
				})

				ThenErrorIsExpected(s, Struct)
			})

			s.Context("Yoda conditions -> x>", func(s *testcase.Spec) {
				type T struct {
					V int `range:"42>"`
				}

				s.Context("within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(-42, 41)}
					})

					ThenNoErrorIsExpected(s, Struct)
				})

				s.Context("not-within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(42, 100)}
					})

					ThenErrorIsExpected(s, Struct)
				})
			})
		})

		s.Context(">= (greater than or equal to)", func(s *testcase.Spec) {
			type T struct {
				V int `range:">=42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(42, 100)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(-42, 41)}
				})

				ThenErrorIsExpected(s, Struct)
			})

			s.Context("Yoda conditions - x<=", func(s *testcase.Spec) {
				type T struct {
					V int `range:"42<="`
				}

				s.Context("within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(42, 100)}
					})

					ThenNoErrorIsExpected(s, Struct)
				})

				s.Context("not-within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(-42, 41)}
					})

					ThenErrorIsExpected(s, Struct)
				})

			})
		})

		s.Context("<= (less than or equal to)", func(s *testcase.Spec) {
			type T struct {
				V int `range:"<=42"`
			}

			s.Context("within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(-42, 42)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.Context("not-within", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.IntBetween(43, 100)}
				})

				ThenErrorIsExpected(s, Struct)
			})

			s.Context("Yoda conditions -> 42>=", func(s *testcase.Spec) {
				type T struct {
					V int `range:"42>="`
				}

				s.Context("within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(-42, 42)}
					})

					ThenNoErrorIsExpected(s, Struct)
				})

				s.Context("not-within", func(s *testcase.Spec) {
					Struct.Let(s, func(t *testcase.T) any {
						return T{V: t.Random.IntBetween(43, 100)}
					})

					ThenErrorIsExpected(s, Struct)
				})

			})
		})

		s.Context("malformed syntax with operators on both side of the value", func(s *testcase.Spec) {
			type T struct {
				V int `range:"<3<"`
			}

			Struct.Let(s, func(t *testcase.T) any {
				return T{V: 3}
			})

			ThenPanicIsExpected(s, Struct)
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

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))

			assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: t.Random.StringNC(1, "defg")}), &got))
		})

		s.Test("subtype", func(t *testcase.T) {
			type STR string
			type T struct {
				V STR `char:"a-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: STR(t.Random.StringNC(1, "abc"))}))
			assert.NoError(t, validate.Value(t.Context(), T{V: STR(t.Random.StringNC(t.Random.IntBetween(3, 7), "abc"))}))

			assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			var got validate.Error
			assert.True(t, errors.As(validate.Value(t.Context(), T{V: STR(t.Random.StringNC(1, "defg"))}), &got))
		})

		s.Test("multi", func(t *testcase.T) {
			type T struct {
				V string `char:"a-c,e-g"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abcefg")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abcefg")}))

			assert.Error(t, validate.Value(t.Context(), T{V: "d"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "dddd"}))
			assert.Error(t, validate.Value(t.Context(), T{V: "avb" + t.Random.StringNC(1, "ijklmnopqrstuvwxyz")}))
		})

		s.Test("no-min", func(t *testcase.T) {
			type T struct {
				V string `char:"-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(3, 7), "abc")}))
			assert.Error(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
		})

		s.Test("no-max", func(t *testcase.T) {
			type T struct {
				V string `char:"d-"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: t.Random.StringNC(t.Random.IntBetween(1, 7), string(iterkit.Collect(iterkit.CharRange('d', 'z'))))}))
			assert.Error(t, validate.Value(t.Context(), T{V: t.Random.StringNC(1, "abc")}))
		})
	})

	s.Describe("rune", func(s *testcase.Spec) {
		s.Test("single char range", func(t *testcase.T) {
			type T struct {
				V rune `char:"a-c"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "abc"))[0]}))
			assert.Error(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
		})
		s.Test("multi char range", func(t *testcase.T) {
			type T struct {
				V rune `char:"a-c,e-g"`
			}

			assert.NoError(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "abcefg"))[0]}))
			assert.Error(t, validate.Value(t.Context(), T{V: []rune(t.Random.StringNC(1, "dhijklmnopqrstuvwxyz"))[0]}))
		})
	})

	s.Test("invalid char tag", func(t *testcase.T) {
		type NonSingleCharMin struct {
			V string `char:"aa-c"`
		}

		assert.Panic(t, func() { validate.Value(t.Context(), NonSingleCharMin{}) })

		type NonSingleCharMax struct {
			V string `char:"a-cc"`
		}

		assert.Panic(t, func() { validate.Value(t.Context(), NonSingleCharMax{}) })
	})
}

type SkipValidateStruct struct {
	ValidateErr error
}

func (v SkipValidateStruct) Validate(context.Context) error {
	return v.ValidateErr
}

func TestSkipValidate(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("skip validate will make the validation call to be skipped on the given value itself", func(t *testcase.T) {
		val := SkipValidateStruct{ValidateErr: t.Random.Error()}
		assert.NoError(t, validate.Value(validate.SkipValidate(t.Context()), val))
		assert.NoError(t, validate.Struct(validate.SkipValidate(t.Context()), val))

		type T struct{ V StructValidatorStub }
		field, value := must.OK2(reflectkit.LookupField(reflect.ValueOf(T{V: StructValidatorStub{ValidateError: t.Random.Error()}}), "V"))
		assert.NoError(t, validate.StructField(validate.SkipValidate(t.Context()), field, value))
	})

	s.Test("skip validate won't make the validation call to be skipped on fields of a struct value", func(t *testcase.T) {
		type T struct{ V SkipValidateStruct }
		expErr := t.Random.Error()

		val := T{V: SkipValidateStruct{ValidateErr: expErr}}
		assert.ErrorIs(t, expErr, validate.Value(validate.SkipValidate(t.Context()), val))
		assert.ErrorIs(t, expErr, validate.Struct(validate.SkipValidate(t.Context()), val))

		type TT struct{ V T }
		field, value := must.OK2(reflectkit.LookupField(reflect.ValueOf(TT{V: val}), "V"))
		assert.ErrorIs(t, expErr, validate.StructField(validate.SkipValidate(t.Context()), field, value))
	})
}

func Test_infRecursionSelfValidation(t *testing.T) {
	t.Run("w validate.Value", func(t *testing.T) {
		assert.NoError(t, validate.Value(t.Context(), CircularValidation{}))
		assert.Error(t, validate.Value(t.Context(), CircularValidation{IntField: 42}))
		assert.Error(t, validate.Value(t.Context(), CircularValidation{unexported: "invalid"}))

	})

	t.Run("w T#Validate", func(t *testing.T) {
		assert.NoError(t, CircularValidation{}.Validate(t.Context()))
		assert.Error(t, CircularValidation{IntField: 42}.Validate(t.Context()))
		assert.Error(t, CircularValidation{unexported: "invalid"}.Validate(t.Context()))
	})
}

type CircularValidation struct {
	IntField int `range:"0..10"`

	unexported StringValidatorStub
}

func (cv CircularValidation) Validate(ctx context.Context) error {
	// validate.Value to utilise the tag based validation
	if err := validate.Value(ctx, cv); err != nil {
		return err
	}
	// validate unexported internal stuff
	if err := validate.Value(ctx, cv.unexported); err != nil {
		return err
	}
	return nil
}

type ExampleStructT1 struct {
	EnumField string `enum:"foo bar baz"`
	IntField  int    `range:"0..10"`
}

func (v ExampleStructT1) Validate(ctx context.Context) error {
	validate.Struct(validate.SkipValidate(ctx), v)
	if v.EnumField == "foo" {

	}
	if v.IntField == 7 {
		return fmt.Errorf("custom")
	}
	return nil
}

func Test_min_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	var Struct = let.Var[any](s, nil)

	s.Context("int", func(s *testcase.Spec) {
		type T struct {
			V int `min:"5"`
		}

		s.When("number is greater or equal compared to min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.IntBetween(5, 100)}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is less compared the min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.IntBetween(-100, 4)}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("uint", func(s *testcase.Spec) {
		type T struct {
			V uint `min:"5"`
		}

		s.When("number is greater or equal compared to min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: uint(t.Random.IntBetween(5, 100))}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is less compared the min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: uint(t.Random.IntBetween(0, 4))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("float", func(s *testcase.Spec) {
		type T struct {
			V float64 `min:"4.5"`
		}

		s.When("number is greater or equal compared to min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: 4.5 + float64(t.Random.IntBetween(0, 100))}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is less compared the min", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: 4.49 - float64(t.Random.IntBetween(0, 100))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})
}

func Test_max_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	var Struct = let.Var[any](s, nil)

	s.Context("int", func(s *testcase.Spec) {
		type T struct {
			V int `max:"5"`
		}

		s.When("number is less or equal compared to max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.IntBetween(-100, 5)}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is greater compared the max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.IntBetween(6, 100)}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("uint", func(s *testcase.Spec) {
		type T struct {
			V uint `max:"5"`
		}

		s.When("number is less or equal compared to max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: uint(t.Random.IntBetween(0, 5))}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is greater compared the max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: uint(t.Random.IntBetween(6, 100))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})

	s.Context("float", func(s *testcase.Spec) {
		type T struct {
			V float64 `max:"4.5"`
		}

		s.When("number is less or equal compared to max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: 4.5 - float64(t.Random.IntBetween(0, 100))}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.When("number is greater compared the max", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: 4.51 + float64(t.Random.IntBetween(0, 100))}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})
}

func Test_length(t *testing.T) {

	s := testcase.NewSpec(t)

	var Struct = let.Var[any](s, nil)

	s.Context("slice", func(s *testcase.Spec) {
		s.Context("equality", func(s *testcase.Spec) {
			type T struct {
				V []int `length:"5"`
			}

			s.When("slice length matches", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Slice(5, t.Random.Int)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length differs", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					length := random.Unique(func() int {
						return t.Random.IntBetween(1, 10)
					}, 5) // anything but 5
					return T{V: random.Slice(length, t.Random.Int)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("range", func(s *testcase.Spec) {
			type T struct {
				V []int `length:"5..10"`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Slice(t.Random.IntBetween(5, 10), t.Random.Int)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := random.Pick(t.Random,
						t.Random.IntBetween(0, 4),
						t.Random.IntBetween(11, 20))
					return T{V: random.Slice(incorrectLength, t.Random.Int)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("open-range", func(s *testcase.Spec) {
			type T struct {
				V []int `length:"5.."`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Slice(t.Random.IntBetween(5, 100), t.Random.Int)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := t.Random.IntBetween(0, 4)
					return T{V: random.Slice(incorrectLength, t.Random.Int)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("comparison-expression", func(s *testcase.Spec) {
			type T struct {
				V []int `length:"<=10"`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Slice(t.Random.IntBetween(0, 10), t.Random.Int)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Slice(t.Random.IntBetween(11, 15), t.Random.Int)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})
	})

	s.Context("chan", func(s *testcase.Spec) {
		type VT = chan int

		var makeBufferedChan = func(t *testcase.T, n int) chan int {
			ch := make(VT, t.Random.IntBetween(n, n+15))
			assert.Within(t, time.Second, func(ctx context.Context) {
				for i := 0; i < n; i++ {
					ch <- t.Random.Int()
				}
			})
			return ch
		}

		s.Context("equality", func(s *testcase.Spec) {
			type T struct {
				V VT `length:"5"`
			}

			s.When("chan length matches", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: makeBufferedChan(t, 5)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("chan length differs", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					length := random.Unique(func() int {
						return t.Random.IntBetween(1, 10)
					}, 5) // anything but 5
					return T{V: makeBufferedChan(t, length)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("range", func(s *testcase.Spec) {
			type T struct {
				V VT `length:"5..10"`
			}

			s.When("chan length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: makeBufferedChan(t, t.Random.IntBetween(5, 10))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("chan length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := random.Pick(t.Random,
						t.Random.IntBetween(0, 4),
						t.Random.IntBetween(11, 20))
					return T{V: makeBufferedChan(t, incorrectLength)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("open-range", func(s *testcase.Spec) {
			type T struct {
				V VT `length:"5.."`
			}

			s.When("chan length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: makeBufferedChan(t, t.Random.IntBetween(5, 100))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("chan length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := t.Random.IntBetween(0, 4)
					return T{V: makeBufferedChan(t, incorrectLength)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("comparison-expression", func(s *testcase.Spec) {
			type T struct {
				V VT `length:"<=10"`
			}

			s.When("chan length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: makeBufferedChan(t, t.Random.IntBetween(0, 10))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("chan length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: makeBufferedChan(t, t.Random.IntBetween(11, 15))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})
	})

	s.Context("map", func(s *testcase.Spec) {

		var randomKV = func(t *testcase.T) func() (string, int) {
			return func() (s string, i int) {
				return t.Random.StringN(7), t.Random.Int()
			}
		}

		s.Context("equality", func(s *testcase.Spec) {
			type T struct {
				V map[string]int `length:"5"`
			}

			s.When("map length matches", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Map(5, randomKV(t))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("map length differs", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					length := random.Unique(func() int {
						return t.Random.IntBetween(1, 10)
					}, 5) // anything but 5
					return T{V: random.Map(length, randomKV(t))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("range", func(s *testcase.Spec) {
			type T struct {
				V map[string]int `length:"5..10"`
			}

			s.When("map length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Map(t.Random.IntBetween(5, 10), randomKV(t))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("map length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := random.Pick(t.Random,
						t.Random.IntBetween(0, 4),
						t.Random.IntBetween(11, 20))
					return T{V: random.Map(incorrectLength, randomKV(t))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("open-range", func(s *testcase.Spec) {
			type T struct {
				V map[string]int `length:"5.."`
			}

			s.When("map length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Map(t.Random.IntBetween(5, 100), randomKV(t))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("map length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := t.Random.IntBetween(0, 4)
					return T{V: random.Map(incorrectLength, randomKV(t))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("comparison-expression", func(s *testcase.Spec) {
			type T struct {
				V map[string]int `length:"<=10"`
			}

			s.When("map length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Map(t.Random.IntBetween(0, 10), randomKV(t))}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("map length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: random.Map(t.Random.IntBetween(11, 15), randomKV(t))}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})
	})

	s.Context("string", func(s *testcase.Spec) {
		s.Context("equality", func(s *testcase.Spec) {
			type T struct {
				V string `length:"5"`
			}

			s.When("slice length matches", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					return T{V: t.Random.StringN(5)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length differs", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					n := random.Unique(func() int { return t.Random.IntBetween(0, 100) }, 5)
					return T{V: t.Random.StringN(n)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("range", func(s *testcase.Spec) {
			type T struct {
				V string `length:"5..10"`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					n := t.Random.IntBetween(5, 10)
					return T{V: t.Random.StringN(n)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := random.Pick(t.Random,
						t.Random.IntBetween(0, 4),
						t.Random.IntBetween(11, 20))
					return T{V: t.Random.StringN(incorrectLength)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("open-range", func(s *testcase.Spec) {
			type T struct {
				V string `length:"5.."`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					n := t.Random.IntBetween(5, 100)
					return T{V: t.Random.StringN(n)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					incorrectLength := t.Random.IntBetween(0, 4)
					return T{V: t.Random.StringN(incorrectLength)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})

		s.Context("comparison-expression", func(s *testcase.Spec) {
			type T struct {
				V string `length:"<=10"`
			}

			s.When("slice length is ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					n := t.Random.IntBetween(0, 10)
					return T{V: t.Random.StringN(n)}
				})

				ThenNoErrorIsExpected(s, Struct)
			})

			s.When("slice length is not ok", func(s *testcase.Spec) {
				Struct.Let(s, func(t *testcase.T) any {
					n := t.Random.IntBetween(11, 100)
					return T{V: t.Random.StringN(n)}
				})

				ThenErrorIsExpected(s, Struct)
			})
		})
	})

	s.Context("len alias", func(s *testcase.Spec) {
		type T struct {
			V string `len:"5"`
		}

		s.Context("OK", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				return T{V: t.Random.StringN(5)}
			})

			ThenNoErrorIsExpected(s, Struct)
		})

		s.Context("NOK", func(s *testcase.Spec) {
			Struct.Let(s, func(t *testcase.T) any {
				n := random.Unique(func() int { return t.Random.IntBetween(0, 10) }, 5)
				return T{V: t.Random.StringN(n)}
			})

			ThenErrorIsExpected(s, Struct)
		})
	})
}

func assertErrorIsValidateError(tb testing.TB, err error) validate.Error {
	tb.Helper()
	assert.Error(tb, err)
	var verr validate.Error
	assert.True(tb, errors.As(err, &verr), "expected a validate error")
	return verr
}

func ThenErrorIsExpected(s *testcase.Spec, Struct testcase.Var[any]) {
	s.H().Helper()

	s.Then("error is expected with validate.Value", func(t *testcase.T) {
		vErr := assertErrorIsValidateError(t, validate.Value(t.Context(), Struct.Get(t)))
		assert.Error(t, vErr.Cause)
	})

	s.Then("error is expected with validate.Struct", func(t *testcase.T) {
		vErr := assertErrorIsValidateError(t, validate.Struct(t.Context(), Struct.Get(t)))
		assert.Error(t, vErr.Cause)
	})

	s.Then("error is expected with validate.StructField", func(t *testcase.T) {
		ctx, field, value := toStructField(t, Struct.Get(t), "V")
		vErr := assertErrorIsValidateError(t, validate.StructField(ctx, field, value))
		assert.Error(t, vErr.Cause)
	})
}

func ThenPanicIsExpected(s *testcase.Spec, Struct testcase.Var[any]) {
	s.H().Helper()

	s.Then("panic is expected with validate.Value", func(t *testcase.T) {
		assert.Panic(t, func() { validate.Value(t.Context(), Struct.Get(t)) })
	})

	s.Then("panic is expected with validate.Struct", func(t *testcase.T) {
		assert.Panic(t, func() { validate.Struct(t.Context(), Struct.Get(t)) })
	})

	s.Then("panic is expected with validate.StructField", func(t *testcase.T) {
		ctx, field, value := toStructField(t, Struct.Get(t), "V")
		assert.Panic(t, func() { validate.StructField(ctx, field, value) })
	})
}

func ThenNoErrorIsExpected(s *testcase.Spec, Struct testcase.Var[any]) {
	s.H().Helper()

	s.Then("error is expected with validate.Value", func(t *testcase.T) {
		assert.NoError(t, validate.Value(t.Context(), Struct.Get(t)))
	})

	s.Then("error is expected with validate.Struct", func(t *testcase.T) {
		assert.NoError(t, validate.Struct(t.Context(), Struct.Get(t)))
	})

	s.Then("error is expected with validate.StructField", func(t *testcase.T) {
		assert.NoError(t, validate.StructField(toStructField(t, Struct.Get(t), "V")))
	})
}

func Test_match_smoke(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		type T struct {
			V string `match:"^foo$"`
		}
		var (
			ok   = T{V: "foo"}
			nok  = T{V: "oof"}
			zero = T{}
		)

		ctx := t.Context()
		assert.NoError(t, validate.Value(ctx, ok))
		assert.Error(t, validate.Value(ctx, nok))
		assert.Error(t, validate.Value(ctx, zero))

		assert.NoError(t, validate.Struct(ctx, ok))
		assert.Error(t, validate.Struct(ctx, nok))
		assert.Error(t, validate.Struct(ctx, zero))

		assert.NoError(t, validate.StructField(toStructField(t, ok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, nok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, zero, "V")))
	})

	t.Run("~string", func(t *testing.T) {
		type S string
		type T struct {
			V S `match:"^foo$"`
		}
		var (
			ok   = T{V: "foo"}
			nok  = T{V: "oof"}
			zero = T{}
		)

		assert.NoError(t, validate.Value(t.Context(), ok))
		assert.Error(t, validate.Value(t.Context(), nok))
		assert.Error(t, validate.Value(t.Context(), zero))

		assert.NoError(t, validate.Struct(t.Context(), ok))
		assert.Error(t, validate.Struct(t.Context(), nok))
		assert.Error(t, validate.Struct(t.Context(), zero))

		assert.NoError(t, validate.StructField(toStructField(t, ok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, nok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, zero, "V")))
	})

	t.Run("[]byte", func(t *testing.T) {
		type T struct {
			V []byte `match:"^foo$"`
		}
		var (
			ok   = T{V: []byte("foo")}
			nok  = T{V: []byte("oof")}
			zero = T{}
		)

		assert.NoError(t, validate.Value(t.Context(), ok))
		assert.Error(t, validate.Value(t.Context(), nok))
		assert.Error(t, validate.Value(t.Context(), zero))

		assert.NoError(t, validate.Struct(t.Context(), ok))
		assert.Error(t, validate.Struct(t.Context(), nok))
		assert.Error(t, validate.Struct(t.Context(), zero))

		assert.NoError(t, validate.StructField(toStructField(t, ok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, nok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, zero, "V")))
	})

	t.Run("~[]byte", func(t *testing.T) {
		type BSlice []byte

		type T struct {
			V BSlice `match:"^foo$"`
		}
		var (
			ok   = T{V: BSlice("foo")}
			nok  = T{V: BSlice("oof")}
			zero = T{}
		)

		assert.NoError(t, validate.Value(t.Context(), ok))
		assert.Error(t, validate.Value(t.Context(), nok))
		assert.Error(t, validate.Value(t.Context(), zero))

		assert.NoError(t, validate.Struct(t.Context(), ok))
		assert.Error(t, validate.Struct(t.Context(), nok))
		assert.Error(t, validate.Struct(t.Context(), zero))

		assert.NoError(t, validate.StructField(toStructField(t, ok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, nok, "V")))
		assert.Error(t, validate.StructField(toStructField(t, zero, "V")))
	})
}

type DeprecatedSelfValidatorMethod struct {
	ValidateErr error
}

func (stub DeprecatedSelfValidatorMethod) Validate() error {
	return stub.ValidateErr
}

func Test_backwardCompability_ValidatorValidate(t *testing.T) {
	t.Run("fallback to deprecated method signature", func(t *testing.T) {
		assert.NoError(t, validate.Value(t.Context(), DeprecatedSelfValidatorMethod{}))

		expErr := rnd.Error()
		gotErr := validate.Value(t.Context(), DeprecatedSelfValidatorMethod{ValidateErr: expErr})

		assert.ErrorIs(t, gotErr, expErr)

		vErr, ok := errorkit.As[validate.Error](gotErr)
		assert.True(t, ok, "expected that got error is a validate.Error type")
		assert.ErrorIs(t, expErr, vErr)
	})
	t.Run("SkipValidate respected", func(t *testing.T) {

	})
}
