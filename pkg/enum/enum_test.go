package enum_test

import (
	"errors"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/validate"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleValidateStruct_string() {
	type ExampleStruct struct {
		V string `enum:"A;B;C;"`
	}

	_ = enum.ValidateStruct(ExampleStruct{V: "A"}) // no error
	_ = enum.ValidateStruct(ExampleStruct{V: "D"}) // has error
}

func ExampleValidateStruct_int() {
	type ExampleStruct struct {
		V int `enum:"2,4,8,16,42,"`
	}

	_ = enum.ValidateStruct(ExampleStruct{V: 42}) // no error
	_ = enum.ValidateStruct(ExampleStruct{V: 24}) // has error
}

func ExampleValidateStruct_float() {
	type ExampleStruct struct {
		V float64 `enum:"2.5;4.2;"`
	}

	_ = enum.ValidateStruct(ExampleStruct{V: 4.2})   // no error
	_ = enum.ValidateStruct(ExampleStruct{V: 24.42}) // has error
}

func ExampleValidateStruct_slice() {
	type ExampleStruct struct {
		V []string `enum:"FOO|BAR|BAZ|"`
	}

	_ = enum.ValidateStruct(ExampleStruct{V: []string{"FOO", "BAR", "BAZ"}}) // no error
	_ = enum.ValidateStruct(ExampleStruct{V: []string{"FOO", "BAB", "BAZ"}}) // has error because of BAB
}

func TestValidateStruct_smoke(t *testing.T) {
	type (
		EmptyEnumExample struct {
			V string `enum:""`
		}
		UnsupportedEnumExample struct {
			V func() `enum:";"`
		}
		InvalidExample struct {
			V int `enum:"hello;world;"`
		}
		StringExample struct {
			V string `enum:"A|B|C|"`
		}
		IntExample struct {
			V int `enum:"42|24|"`
		}
		Int8Example struct {
			V int8 `enum:"42,24,"`
		}
		Int16Example struct {
			V int16 `enum:"42;24;"`
		}
		Int32Example struct {
			V int32 `enum:"42/24/"`
		}
		Int64Example struct {
			V int64 `enum:"42/24/"`
		}
		UIntExample struct {
			V uint `enum:"42|24|"`
		}
		UInt8Example struct {
			V uint8 `enum:"42,24,"`
		}
		UInt16Example struct {
			V uint16 `enum:"42;24;"`
		}
		UInt32Example struct {
			V uint32 `enum:"42/24/"`
		}
		UInt64Example struct {
			V uint64 `enum:"42/24/"`
		}
		Float32Example struct {
			V float32 `enum:"42.24 24.42"`
		}
		Float64Example struct {
			V float64 `enum:"42.24;24.42;"`
		}
		BoolExample struct {
			V bool `enum:"true;"`
		}
		SubStringType        string
		StringSubTypeExample struct {
			V SubStringType `enum:"A;B;C;"`
		}
		SliceExample struct {
			V []string `enum:"A;B;C;"`
		}
		DefaultValueSeparatorDoesNotRequireLastComma struct {
			V string `enum:"foo,bar,baz"`
		}
		DefaultCommaSeparatorTrimsSpaces struct {
			V string `enum:"foo , bar , baz"`
		}
		DefaultSeperationWithSpaces struct {
			V float32 `enum:"42.24  24.42"`
		}
		DefaultSeperationWithSpacesAndTrimmingRequired struct {
			V string `enum:"foo  bar baz "`
		}
		ExplicitSpaceSeperation struct {
			V string `enum:"foo bar baz "`
		}
	)

	type Case struct {
		V     any
		IsErr bool
		Test  func(t *testcase.T)
	}
	cases := map[string]Case{
		"when the last character ends with a non-special character, and space is used to seperate the enum list": {
			V:     DefaultSeperationWithSpaces{V: 24.42},
			IsErr: false,
		},

		"when the last character ends with a non-special character, and space is used to seperate the enum list with extra spaces between and after": {
			Test: func(t *testcase.T) {
				field, _, ok := reflectkit.LookupField(reflect.ValueOf(DefaultSeperationWithSpacesAndTrimmingRequired{}), "V")
				assert.True(t, ok, "incorrect test code!!!")

				enums := slicekit.Map(enum.ReflectValues(field), reflect.Value.String)
				t.LogPretty(enums)
				assert.ContainsExactly(t, enums, []string{"foo", "bar", "baz"})

				assert.NoError(t, enum.Validate(DefaultSeperationWithSpacesAndTrimmingRequired{V: "foo"}))
				assert.NoError(t, enum.Validate(DefaultSeperationWithSpacesAndTrimmingRequired{V: "bar"}))
				assert.NoError(t, enum.Validate(DefaultSeperationWithSpacesAndTrimmingRequired{V: "baz"}))
			},
		},

		"when the last character ends with a non-special character, and comma is used to seperate, then spaces are trimmed": {
			Test: func(t *testcase.T) {
				assert.NoError(t, enum.Validate(DefaultCommaSeparatorTrimsSpaces{V: "bar"}))

				field, ok := reflectkit.TypeOf[DefaultCommaSeparatorTrimsSpaces]().FieldByName("V")
				assert.True(t, ok, "test is inccorrect, please fix it")

				enums, err := enum.ReflectValuesOfStructField(field)
				assert.NoError(t, err)

				strEnums := slicekit.Map(enums, reflect.Value.String)
				assert.ContainsExactly(t, []string{"foo", "bar", "baz"}, strEnums)
			},
		},

		"when the last character ends with a non-special character, and space is used to seperate, then spaces are trimmed": {
			Test: func(t *testcase.T) {
				assert.NoError(t, enum.Validate(DefaultCommaSeparatorTrimsSpaces{V: "bar"}))
			},
		},

		"when the last character explicitly is a space symbol": {
			V:     ExplicitSpaceSeperation{V: "bar"},
			IsErr: false,
		},

		"on non struct value type, validation fails": {
			V:     "Hello, world!",
			IsErr: true,
		},

		"on empty enum list, everything is accepted": {
			V:     EmptyEnumExample{V: "foo/bar/baz"},
			IsErr: false,
		},

		"on unsupported enum list, error returned": {
			V:     UnsupportedEnumExample{V: func() {}},
			IsErr: true,
		},

		"on invalid enumerator type, error returned": {
			V:     InvalidExample{V: 42},
			IsErr: true,
		},

		"on sub type, when value match the enum list": {
			V:     StringSubTypeExample{V: "A"},
			IsErr: false,
		},
		"on sub type, when value doesn't match the enum list": {
			V:     StringSubTypeExample{V: "foo"},
			IsErr: true,
		},

		"bool - match enum list": {
			V:     BoolExample{V: true},
			IsErr: false,
		},
		"bool - doesn't match enum list": {
			V:     BoolExample{V: false},
			IsErr: true,
		},

		"string - match enum list - enum pos 1": {
			V:     StringExample{V: "A"},
			IsErr: false,
		},
		"string - match enum list - enum pos 2": {
			V:     StringExample{V: "B"},
			IsErr: false,
		},
		"string - doesn't match enum list - invalid value": {
			V:     StringExample{V: "128"},
			IsErr: true,
		},
		"string - doesn't match enum list - zero value, when zero is not registered as valid value": {
			V:     StringExample{},
			IsErr: true,
		},

		"int - match enum list": {
			V:     IntExample{V: 42},
			IsErr: false,
		},
		"int - doesn't match enum list": {
			V:     IntExample{V: 128},
			IsErr: true,
		},

		"int8 - match enum list": {
			V:     Int8Example{V: 42},
			IsErr: false,
		},
		"int8 - doesn't match enum list": {
			V:     Int8Example{V: 16},
			IsErr: true,
		},

		"int16 - match enum list": {
			V:     Int16Example{V: 42},
			IsErr: false,
		},
		"int16 - doesn't match enum list": {
			V:     Int16Example{V: 128},
			IsErr: true,
		},

		"int32 - match enum list": {
			V:     Int32Example{V: 42},
			IsErr: false,
		},
		"int32 - doesn't match enum list": {
			V:     Int32Example{V: 128},
			IsErr: true,
		},

		"int64 - match enum list": {
			V:     Int64Example{V: 42},
			IsErr: false,
		},
		"int64 - doesn't match enum list": {
			V:     Int64Example{V: 128},
			IsErr: true,
		},

		"uint - match enum list": {
			V:     UIntExample{V: 42},
			IsErr: false,
		},
		"uint - doesn't match enum list": {
			V:     UIntExample{V: 128},
			IsErr: true,
		},

		"uint8 - match enum list": {
			V:     UInt8Example{V: 42},
			IsErr: false,
		},
		"uint8 - doesn't match enum list": {
			V:     UInt8Example{V: 16},
			IsErr: true,
		},

		"uint16 - match enum list": {
			V:     UInt16Example{V: 42},
			IsErr: false,
		},
		"uint16 - doesn't match enum list": {
			V:     UInt16Example{V: 128},
			IsErr: true,
		},

		"uint32 - match enum list": {
			V:     UInt32Example{V: 42},
			IsErr: false,
		},
		"uint32 - doesn't match enum list": {
			V:     UInt32Example{V: 128},
			IsErr: true,
		},

		"uint64 - match enum list": {
			V:     UInt64Example{V: 42},
			IsErr: false,
		},
		"uint64 - doesn't match enum list": {
			V:     UInt64Example{V: 128},
			IsErr: true,
		},

		"float32 - match enum list": {
			V:     Float32Example{V: 42.24},
			IsErr: false,
		},
		"float32 - doesn't match enum list": {
			V:     Float32Example{V: 42.42},
			IsErr: true,
		},

		"float64 - match enum list": {
			V:     Float64Example{V: 42.24},
			IsErr: false,
		},
		"float64 - doesn't match enum list": {
			V:     Float64Example{V: 42.42},
			IsErr: true,
		},

		"slice - values match the enum list": {
			V:     SliceExample{V: []string{"A", "C"}},
			IsErr: false,
		},
		"slice - values doesn't match the enum list": {
			V:     SliceExample{V: []string{"A", "foo"}},
			IsErr: true,
		},
		"if comma used for enum value seperation, last comma can be ommited": {
			V:     DefaultValueSeparatorDoesNotRequireLastComma{V: "bar"},
			IsErr: false,
		},
	}

	testcase.TableTest(t, cases, func(t *testcase.T, c Case) {
		if c.Test != nil {
			c.Test(t)
			return
		}

		gotErr := enum.ValidateStruct(c.V)

		if c.IsErr {
			t.Must.Error(gotErr)
		} else {
			t.Must.NoError(gotErr)
		}

	})

	t.Run("test position", func(t *testing.T) {
		assert.Error(t, enum.ValidateStruct(StringExample{}))
		assert.Error(t, enum.ValidateStruct(StringExample{V: "42"}))
		assert.NoError(t, enum.ValidateStruct(StringExample{V: "A"}))
		assert.NoError(t, enum.ValidateStruct(StringExample{V: "B"}))
		assert.NoError(t, enum.ValidateStruct(StringExample{V: "C"}))
	})
}

func TestRegister(t *testing.T) {
	type X string
	const (
		C1 X = "C1"
		C2 X = "C2"
		C3 X = "C3"
	)

	unregister := enum.Register[X](C1, C2, C3)
	defer unregister()

	t.Run("exported field", func(t *testing.T) {
		type T struct{ V X }

		assert.NoError(t, enum.ValidateStruct(T{V: C1}))
		assert.NoError(t, enum.ValidateStruct(T{V: C2}))
		assert.NoError(t, enum.ValidateStruct(T{V: C3}))
		assert.ErrorIs(t, enum.ErrInvalid, enum.ValidateStruct(T{V: "C4"}))

		unregister()
		assert.NoError(t, enum.ValidateStruct(T{V: "C4"}))
	})
}

func ExampleValues() {
	type T string
	const (
		V1 T = "C1"
		V2 T = "C2"
		V3 T = "C3"
	)
	enum.Register[T](V1, V2, V3)

	enum.Values[T]() // []T{"C1", "C2", "C3"}
}

func TestValues(t *testing.T) {
	t.Run("no enum value member is registered", func(t *testing.T) {
		type T struct{}
		assert.Empty(t, enum.Values[T]())
	})
	t.Run("type with registered enum value members", func(t *testing.T) {
		type T string
		const (
			V1 T = "C1"
			V2 T = "C2"
			V3 T = "C3"
		)

		unregister := enum.Register[T](V1, V2, V3)
		defer unregister()

		t.Log("after register, enum member values are available")
		assert.ContainsExactly(t, []T{V1, V2, V3}, enum.Values[T]())

		t.Log("after unregister, enum member values are no longer available")
		unregister()
		assert.Empty(t, enum.Values[T]())
	})
}

func ExampleReflectValues() {
	type T string
	const (
		V1 T = "C1"
		V2 T = "C2"
		V3 T = "C3"
	)
	enum.Register[T](V1, V2, V3)

	enum.ReflectValues(reflect.TypeOf((*T)(nil)).Elem()) // []{"C1", "C2", "C3"}
}

func TestReflectValues(t *testing.T) {
	t.Run("no enum value member is registered", func(t *testing.T) {
		type T struct{}
		assert.Empty(t, enum.ReflectValues(reflectkit.TypeOf[T]()))
	})
	t.Run("type with registered enum value members", func(t *testing.T) {
		type T string
		const (
			V1 T = "C1"
			V2 T = "C2"
			V3 T = "C3"
		)

		unregister := enum.Register[T](V1, V2, V3)
		defer unregister()

		t.Log("after register, enum member values are available")
		assert.ContainsExactly(t,
			[]reflect.Value{reflect.ValueOf(V1), reflect.ValueOf(V2), reflect.ValueOf(V3)},
			enum.ReflectValues(reflectkit.TypeOf[T]()))

		t.Log("after unregister, enum member values are no longer available")
		unregister()
		assert.Empty(t, enum.ReflectValues(reflectkit.TypeOf[T]()))
	})
	t.Run("StructField", func(t *testing.T) {
		type T string
		const (
			V1 T = "C1"
			V2 T = "C2"
			V3 T = "C3"
		)
		unregister := enum.Register[T](V1, V2, V3)
		defer unregister()

		type MyStruct struct {
			Foo T `enum:"v1,v2,v3,"`
			Bar T
		}

		msrv := reflect.ValueOf(MyStruct{})

		fooSF, ok := msrv.Type().FieldByName("Foo")
		assert.True(t, ok)

		assert.Equal(t,
			slicekit.Map(enum.ReflectValues(fooSF), reflect.Value.String),
			[]string{"v1", "v2", "v3"})

		barSF, ok := msrv.Type().FieldByName("Bar")
		assert.True(t, ok)

		assert.Equal(t,
			slicekit.Map(enum.ReflectValues(barSF), reflect.Value.String),
			[]string{"C1", "C2", "C3"})
	})
}

func ExampleValidate() {
	type T string
	const (
		V1 T = "C1"
		V2 T = "C2"
		V3 T = "C3"
	)
	enum.Register[T](V1, V2, V3)

	_ = enum.Validate(V1)         // nil
	_ = enum.Validate(V2)         // nil
	_ = enum.Validate(V3)         // nil
	_ = enum.Validate[T](T("C4")) // enum.Err
}

func TestValidate(t *testing.T) {
	type T string
	const (
		V1 T = "C1"
		V2 T = "C2"
		V3 T = "C3"
	)
	defer enum.Register[T](V1, V2, V3)()

	t.Run("when value is an enumerator", func(t *testing.T) {
		assert.NoError(t, enum.Validate(V1))
		assert.NoError(t, enum.Validate(V2))
		assert.NoError(t, enum.Validate(V3))
	})

	t.Run("when value is not a valid enumerator", func(t *testing.T) {
		assert.Error(t, enum.Validate[T](T("C42")))
	})

	t.Run("when value is a valid enum wrapped in an interface", func(t *testing.T) {
		var v any = V2
		assert.NoError(t, enum.Validate(v))
	})

	t.Run("when value is nil", func(t *testing.T) {
		assert.NoError(t, enum.Validate[any](nil))
	})

	t.Run("when value is a pointer to an enum member then no error is expected", func(t *testing.T) {
		assert.NoError(t, enum.Validate[*T](pointer.Of(V1)))
	})

	t.Run("when pointer of interface type is passed, with a valid enum member value", func(t *testing.T) {
		assert.NoError(t, enum.Validate[*any](pointer.Of[any](V1)))
	})

	t.Run("when value is a pointer to not an enum member", func(t *testing.T) {
		var v *T
		v = pointer.Of[T]("C42")
		assert.Error(t, enum.Validate[*T](v))
	})

	t.Run("when value is a nil pointer of an enum type then no error is expected", func(t *testing.T) {
		assert.NoError(t, enum.Validate[*T](nil))
	})

	t.Run("when type argument is any but the value's type has registered enum", func(t *testing.T) {
		assert.NoError(t, enum.Validate[any](V1))
		assert.Error(t, enum.Validate[any](T("C42")))
	})
}

func TestReflectValidate(t *testing.T) {
	s := testcase.NewSpec(t)

	type T string
	const (
		V1 T = "C1"
		V2 T = "C2"
		V3 T = "C3"
	)
	defer enum.Register[T](V1, V2, V3)()

	s.Test("when value is an enumerator", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(reflect.ValueOf(V1), enum.Type[T]()))
		assert.NoError(t, enum.ReflectValidate(reflect.ValueOf(V2), enum.Type[T]()))
		assert.NoError(t, enum.ReflectValidate(reflect.ValueOf(V3), enum.Type[T]()))
	})

	s.Test("when value is not a valid enumerator", func(t *testcase.T) {
		assert.Error(t, enum.ReflectValidate(reflect.ValueOf(T("C42")), enum.Type[T]()))
	})

	s.Test("when value is a valid enum wrapped in an interface", func(t *testcase.T) {
		var v any = V2
		assert.NoError(t, enum.ReflectValidate(v, enum.Type[any]()))
	})

	s.Test("when value is nil", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(nil, enum.Type[any]()))
	})

	s.Test("when value is a pointer to an enum member then no error is expected", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(pointer.Of(V1), enum.Type[*T]()))
	})

	s.Test("when type is nil, value's type is used", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(V1))
		assert.ErrorIs(t, enum.ReflectValidate(T("C4")), enum.ErrInvalid)
	})

	s.Test("when pointer of interface type is passed, with a valid enum member value", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(pointer.Of[any](V1), enum.Type[*any]()))
	})

	s.Test("when value is a pointer to not an enum member", func(t *testcase.T) {
		var v *T
		v = pointer.Of[T]("C42")
		assert.Error(t, enum.ReflectValidate(v, enum.Type[*T]()))
	})

	s.Test("when value is a nil pointer of an enum type then no error is expected", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(nil, enum.Type[*T]()))
	})

	s.Test("when type argument is an 'any' interface but the value's type has registered enum", func(t *testcase.T) {
		assert.NoError(t, enum.ReflectValidate(V1, enum.Type[any]()))
		assert.Error(t, enum.ReflectValidate(T("C42"), enum.Type[any]()))
	})
}

func TestReflectValuesOfStructField(t *testing.T) {
	t.Run("no enum", func(t *testing.T) {
		type X struct {
			A int
		}

		T := reflectkit.TypeOf[X]()
		field, ok := T.FieldByName("A")
		assert.True(t, ok)

		got, err := enum.ReflectValuesOfStructField(field)
		assert.NoError(t, err)
		assert.Equal(t, len(got), 0)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		T := reflectkit.TypeOf[X]()
		field, ok := T.FieldByName("A")
		assert.True(t, ok)

		_, err := enum.ReflectValuesOfStructField(field)
		assert.Error(t, err)
	})
	t.Run("enum defined in the field tag", func(t *testing.T) {
		type X struct {
			A string `enum:"foo,bar,baz,"`
		}

		T := reflectkit.TypeOf[X]()
		field, ok := T.FieldByName("A")
		assert.True(t, ok)

		got, err := enum.ReflectValuesOfStructField(field)
		assert.NoError(t, err)
		assert.Equal(t, len(got), 3)
		assert.OneOf(t, got, func(t testing.TB, v reflect.Value) {
			assert.Equal(t, v.String(), "foo")
		})
	})
	t.Run("enum registered to the field type", func(t *testing.T) {
		type FieldType string
		defer enum.Register[FieldType]("foo", "bar", "baz")()
		type X struct {
			A FieldType
		}

		T := reflectkit.TypeOf[X]()
		field, ok := T.FieldByName("A")
		assert.True(t, ok)

		got, err := enum.ReflectValuesOfStructField(field)
		assert.NoError(t, err)
		assert.Equal(t, len(got), 3)
		assert.OneOf(t, got, func(t testing.TB, v reflect.Value) {
			assert.Equal(t, v.String(), "foo")
		})
	})
}

func TestValidateStructField(t *testing.T) {
	t.Run("no enum", func(t *testing.T) {
		type X struct {
			A int
		}

		val := reflect.ValueOf(X{A: rnd.Int()})

		sf, field, ok := reflectkit.LookupField(val, "A")
		assert.True(t, ok)

		err := enum.ValidateStructField(sf, field)
		assert.NoError(t, err)
	})
	t.Run("enum in the field is invalid", func(t *testing.T) {
		type X struct {
			A int `enum:"invalid,"`
		}

		val := reflect.ValueOf(X{})
		sf, field, ok := reflectkit.LookupField(val, "A")
		assert.True(t, ok)

		err := enum.ValidateStructField(sf, field)
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

			err := enum.ValidateStructField(sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick(rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := enum.ValidateStructField(sf, field)
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

			err := enum.ValidateStructField(sf, field)
			assert.Error(t, err)
		})

		t.Run("valid value", func(t *testing.T) {
			val := reflect.ValueOf(X{A: random.Pick[FieldType](rnd, "foo", "bar", "baz")})
			sf, field, ok := reflectkit.LookupField(val, "A")
			assert.True(t, ok)

			err := enum.ValidateStructField(sf, field)
			assert.NoError(t, err)
		})
	})
}

func TestValidateStruct_noEnum(t *testing.T) {
	type T struct {
		V string `env:"VAL"`
	}

	var v T

	assert.NoError(t, enum.ValidateStruct(v))
}

func TestRegister_exceptions(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		assert.Panic(t, func() { enum.Register[string]("foo") })
		assert.Panic(t, func() { enum.Register[uint](42) })
		assert.Panic(t, func() { enum.Register[uint8](42) })
		assert.Panic(t, func() { enum.Register[uint16](42) })
		assert.Panic(t, func() { enum.Register[uint32](42) })
		assert.Panic(t, func() { enum.Register[uint64](42) })
		assert.Panic(t, func() { enum.Register[int](42) })
		assert.Panic(t, func() { enum.Register[int8](42) })
		assert.Panic(t, func() { enum.Register[int16](42) })
		assert.Panic(t, func() { enum.Register[int32](42) })
		assert.Panic(t, func() { enum.Register[int64](42) })
		assert.Panic(t, func() { enum.Register[float32](42.0) })
		assert.Panic(t, func() { enum.Register[float64](42.0) })
		assert.Panic(t, func() { enum.Register[complex64](complex(1, 2)) })
		assert.Panic(t, func() { enum.Register[complex128](complex(1, 2)) })
		assert.Panic(t, func() { enum.Register[bool](true) })
		assert.Panic(t, func() { enum.Register[rune]('a') }) // rune is an alias for int32
		assert.Panic(t, func() { enum.Register[byte](255) }) // byte is an alias for uint8
	})

	t.Run("control", func(t *testing.T) {
		type MyString string
		assert.NotPanic(t, func() { enum.Register[MyString]("foo")() })
		type MyInt int
		assert.NotPanic(t, func() { enum.Register[MyInt](42)() })
		type MyFloat32 float32
		assert.NotPanic(t, func() { enum.Register[MyFloat32](42.42)() })
		type MyFloat64 float64
		assert.NotPanic(t, func() { enum.Register[MyFloat64](42.42)() })
		type MyBool bool
		assert.NotPanic(t, func() { enum.Register[MyBool](false)() })
	})
}

func TestStruct_worksWithReflectValue(t *testing.T) {
	type T struct {
		V string `enum:"foo bar baz"`
	}
	t.Run("happy", func(t *testing.T) {
		var v = T{V: random.Pick(rnd, "foo", "bar", "baz")}
		assert.NoError(t, enum.ValidateStruct(v))
		assert.NoError(t, enum.ValidateStruct(reflect.ValueOf(v)))
	})
	t.Run("rainy", func(t *testing.T) {
		var v = T{V: random.Pick(rnd, "qux", "quux", "corge", "grault", "garply")}
		assert.ErrorIs(t, enum.ErrInvalid, enum.ValidateStruct(v))
		assert.ErrorIs(t, enum.ErrInvalid, enum.ValidateStruct(reflect.ValueOf(v)))
	})
}

func TestSTructField_tagTakesPriorityOverType(t *testing.T) {
	t.Log("tag validation takes priority over the type's enums")

	type E string
	defer enum.Register[E]("foo", "bar", "baz")()

	type T struct {
		V E `enum:"hello world"`
	}

	var v T
	v.V = "hello"

	assert.NoError(t, enum.ValidateStruct(v))
	sf1, sv1 := must.OK2(reflectkit.LookupField(reflect.ValueOf(T{V: "hello"}), "V"))
	assert.NoError(t, validate.StructField(t.Context(), sf1, sv1))

	sf2, sv2 := must.OK2(reflectkit.LookupField(reflect.ValueOf(T{V: "foo"}), "V"))
	err := validate.StructField(t.Context(), sf2, sv2)
	assert.ErrorIs(t, err, enum.ErrInvalid)
	var verr validate.Error
	assert.True(t, errors.As(err, &verr))
	assert.ErrorIs(t, verr.Cause, enum.ErrInvalid)
}
