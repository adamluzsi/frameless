package reflectkit_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func TestTypeOf(t *testing.T) {
	t.Run("type by value", func(t *testing.T) {
		var str string
		assert.Equal(t, reflect.String, reflectkit.TypeOf(str).Kind())
	})

	t.Run("type by type argument", func(t *testing.T) {
		assert.Equal(t, reflect.Int32, reflectkit.TypeOf[int32]().Kind())
	})

	t.Run("both type argument and value is supplies", func(t *testing.T) {
		T := reflectkit.TypeOf[testent.Fooer](testent.Foo{})

		assert.Equal(t, T.String(), reflect.TypeOf((*testent.Fooer)(nil)).Elem().String())
	})

	t.Run("when type argument is empty interface", func(t *testing.T) {
		t.Run("and value is supplied too, then the value's type is returned", func(t *testing.T) {
			T := reflectkit.TypeOf[any](testent.Foo{})

			assert.Equal(t, T.String(), reflect.TypeOf((*testent.Foo)(nil)).Elem().String())
		})

		t.Run("and no value is supplied, then the type argument is used", func(t *testing.T) {
			T := reflectkit.TypeOf[any]()

			assert.Equal(t, T.String(), reflect.TypeOf((*any)(nil)).Elem().String())
		})

		t.Run("and the value is a nil, then the type arguent is used", func(t *testing.T) {
			T := reflectkit.TypeOf[any](nil)

			assert.Equal(t, T.String(), reflect.TypeOf((*any)(nil)).Elem().String())
		})

		t.Run("and the first value is a nil, but the second value is not nil, then the type arguent is used", func(t *testing.T) {
			T := reflectkit.TypeOf[any](nil, testent.Foo{})

			assert.Equal(t, T.String(), reflect.TypeOf((*testent.Foo)(nil)).Elem().String())
		})
	})
}

func TestBaseTypeOf(t *testing.T) {
	subject := func(obj interface{}) reflect.Type {
		return reflectkit.BaseTypeOf(obj)
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Name()
	})

	expectedValueType := reflect.TypeOf(StructObject{})

	plainStruct := StructObject{}
	ptrToStruct := &plainStruct
	ptrToPtr := &ptrToStruct

	assert.Equal(t, expectedValueType, subject(plainStruct))
	assert.Equal(t, expectedValueType, subject(ptrToStruct))
	assert.Equal(t, expectedValueType, subject(ptrToPtr))
}

func TestBaseValueOf(t *testing.T) {
	subject := func(input interface{}) reflect.Value {
		return reflectkit.BaseValueOf(input)
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Type().Name()
	})

	expectedValue := reflect.ValueOf(StructObject{})
	expectedValueType := expectedValue.Type()

	plainStruct := StructObject{}
	ptrToStruct := &plainStruct
	ptrToPtr := &ptrToStruct

	assert.Equal(t, expectedValueType, subject(plainStruct).Type())
	assert.Equal(t, expectedValueType, subject(ptrToStruct).Type())
	assert.Equal(t, expectedValueType, subject(ptrToPtr).Type())
}

func TestBaseValue(t *testing.T) {
	subject := func(input interface{}) reflect.Value {
		return reflectkit.BaseValue(reflect.ValueOf(input))
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Type().Name()
	})

	t.Run("invalid", func(t *testing.T) {
		invalid := reflect.Value{}
		assert.Equal(t, invalid, reflectkit.BaseValue(invalid))
	})

	t.Run("pointer", func(t *testing.T) {
		expectedValue := reflect.ValueOf(StructObject{})
		expectedValueType := expectedValue.Type()

		plainStruct := StructObject{}
		ptrToStruct := &plainStruct
		ptrToPtr := &ptrToStruct

		assert.Equal(t, expectedValueType, subject(plainStruct).Type())
		assert.Equal(t, expectedValueType, subject(ptrToStruct).Type())
		assert.Equal(t, expectedValueType, subject(ptrToPtr).Type())
	})

	t.Run("interface", func(t *testing.T) {
		// arrange
		intf := reflect.New(reflectkit.TypeOf[InterfaceObject]()).Elem()
		exp := reflect.ValueOf(StructObject{V: rnd.String()})
		intf.Set(exp)
		// act
		got := reflectkit.BaseValue(intf)
		// assert
		assert.Equal(t, exp.Type().Kind(), got.Kind())
		assert.Equal(t, exp.Type(), got.Type())
		assert.Equal(t, exp.Interface(), got.Interface())
		assert.Equal(t, exp.Type().String(), got.Type().String(), "hotfix")
	})

}

func MustCast[T any](tb testing.TB, exp T, val any) {
	got, ok := reflectkit.Cast[T](val)
	assert.True(tb, ok)
	assert.Equal(tb, exp, got)
}

func TestCast(t *testing.T) {
	t.Run("when it is convertable", func(t *testing.T) {
		got, ok := reflectkit.Cast[int64](int32(42))
		assert.True(t, ok)
		assert.Equal[int64](t, 42, got)
	})

	t.Run("when it is not convertable", func(t *testing.T) {
		got, ok := reflectkit.Cast[int64]("42")
		assert.False(t, ok)
		assert.Equal[int64](t, 0, got)
	})

	t.Run("smoke", func(t *testing.T) {
		MustCast[int](t, int(42), int(42))
		MustCast[int8](t, int8(42), int(42))
		MustCast[int16](t, int16(42), int(42))
		MustCast[int32](t, int32(42), int(42))
		MustCast[int64](t, int64(42), int(42))
		MustCast[int64](t, int64(42), int32(42))
		MustCast[int64](t, int64(42), int16(42))
		got, ok := reflectkit.Cast[int](string("42"))
		assert.False(t, ok, "cast was expected to fail")
		assert.Empty(t, got)
	})
}

type SampleType struct{}

func TestFullyQualifiedName(t *testing.T) {
	subject := reflectkit.FullyQualifiedName

	SpecForPrimitiveNames(t, subject)

	t.Run("when given struct is from different package than the current one", func(t *testing.T) {
		o := SampleType{}

		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".SampleType`, subject(o))
	})

	t.Run("when given object is an interface", func(t *testing.T) {
		var i InterfaceObject = &StructObject{}

		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(i))
	})

	t.Run("when given object is a struct", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(StructObject{}))
	})

	t.Run("when given object is a pointer of a struct", func(t *testing.T) {
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(&StructObject{}))
	})

	t.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
		o := &StructObject{}
		assert.Equal(t, `**"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(&o))
	})

	t.Run("when the given object is a reflect type", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(reflectkit.TypeOf[StructObject]()))
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(reflectkit.TypeOf[*StructObject]()))
		assert.Equal(t, `**"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(reflectkit.TypeOf[**StructObject]()))
	})

	t.Run("when the given object is a reflect value", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(reflect.ValueOf(StructObject{})))
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(reflect.ValueOf(&StructObject{})))
	})
}

func SpecForPrimitiveNames(t *testing.T, subject func(entity interface{}) string) {
	t.Run("when given object is a bool", func(t *testing.T) {
		assert.Equal(t, "bool", subject(true))
	})

	t.Run("when given object is a string", func(t *testing.T) {
		assert.Equal(t, "string", subject(`42`))
	})

	t.Run("when given object is a int", func(t *testing.T) {
		assert.Equal(t, "int", subject(int(42)))
	})

	t.Run("when given object is a int8", func(t *testing.T) {
		assert.Equal(t, "int8", subject(int8(42)))
	})

	t.Run("when given object is a int16", func(t *testing.T) {
		assert.Equal(t, "int16", subject(int16(42)))
	})

	t.Run("when given object is a int32", func(t *testing.T) {
		assert.Equal(t, "int32", subject(int32(42)))
	})

	t.Run("when given object is a int64", func(t *testing.T) {
		assert.Equal(t, "int64", subject(int64(42)))
	})

	t.Run("when given object is a uintptr", func(t *testing.T) {
		assert.Equal(t, "uintptr", subject(uintptr(42)))
	})

	t.Run("when given object is a uint", func(t *testing.T) {
		assert.Equal(t, "uint", subject(uint(42)))
	})

	t.Run("when given object is a uint8", func(t *testing.T) {
		assert.Equal(t, "uint8", subject(uint8(42)))
	})

	t.Run("when given object is a uint16", func(t *testing.T) {
		assert.Equal(t, "uint16", subject(uint16(42)))
	})

	t.Run("when given object is a uint32", func(t *testing.T) {
		assert.Equal(t, "uint32", subject(uint32(42)))
	})

	t.Run("when given object is a uint64", func(t *testing.T) {
		assert.Equal(t, "uint64", subject(uint64(42)))
	})

	t.Run("when given object is a float32", func(t *testing.T) {
		assert.Equal(t, "float32", subject(float32(42)))
	})

	t.Run("when given object is a float64", func(t *testing.T) {
		assert.Equal(t, "float64", subject(float64(42)))
	})

	t.Run("when given object is a complex64", func(t *testing.T) {
		assert.Equal(t, "complex64", subject(complex64(42)))
	})

	t.Run("when given object is a complex128", func(t *testing.T) {
		assert.Equal(t, "complex128", subject(complex128(42)))
	})
}

func TestIsEmpty(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Var[any]{ID: `input value`}
	subject := func(t *testcase.T) bool {
		return reflectkit.IsEmpty(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an pointer to an zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := ""
			return &v
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an pointer to non zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := "Hello, world!"
			return &v
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var slice []string
			return slice
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an empty slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an populated slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{"foo", "bar", "baz"}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m map[string]struct{}
			return m
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an empty map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an populated map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{
				"foo": {},
				"bar": {},
				"baz": {},
			}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m chan struct{}
			return m
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an initialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return make(chan struct{})
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var fn func()
			return fn
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an initialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return func() {}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})
}

func TestIsZero(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Var[any]{ID: `input value`}
	subject := func(t *testcase.T) bool {
		return reflectkit.IsZero(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an pointer to an zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := ""
			return &v
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an pointer to non zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := "Hello, world!"
			return &v
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var slice []string
			return slice
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an zero slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var v []string
			return v
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an empty slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{}
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an populated slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{"foo", "bar", "baz"}
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m map[string]struct{}
			return m
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an zero map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var v map[string]struct{}
			return v
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an empty map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{}
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an populated map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{
				"foo": {},
				"bar": {},
				"baz": {},
			}
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m chan struct{}
			return m
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an initialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return make(chan struct{})
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var fn func()
			return fn
		})

		s.Then(`it will be reported as zero`, func(t *testcase.T) {
			assert.True(t, subject(t))
		})
	})

	s.When(`value is an initialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return func() {}
		})

		s.Then(`it will be reported as non-zero`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})
}

func TestIsNil(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Let[any](s, nil)
	act := func(t *testcase.T) bool {
		return reflectkit.IsNil(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`value is an pointer to an zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := ""
			return &v
		})

		s.Then(`it will be not nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an pointer to non zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := "Hello, world!"
			return &v
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an uninitialized slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var slice []string
			return slice
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`value is an empty slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{}
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an populated slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{"foo", "bar", "baz"}
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an uninitialized map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m map[string]struct{}
			return m
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`value is an empty map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{}
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an populated map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{
				"foo": {},
				"bar": {},
				"baz": {},
			}
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an uninitialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m chan struct{}
			return m
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`value is an initialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return make(chan struct{})
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})

	s.When(`value is an uninitialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var fn func()
			return fn
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`value is an initialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return func() {}
		})

		s.Then(`it will be reported as non-nil`, func(t *testcase.T) {
			assert.Must(t).False(act(t))
		})
	})
}

func TestLink(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		src = testcase.Var[any]{ID: "src"}
		ptr = testcase.Var[any]{ID: "ptr"}
	)
	subject := func(t *testcase.T) error {
		return reflectkit.Link(src.Get(t), ptr.Get(t))
	}

	andPtrPointsToAEmptyInterface := func(s *testcase.Spec) {
		s.And(`ptr points to an empty interface type`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				var i interface{}
				return &i
			})

			s.Then(`it will link the value`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))
				assert.Equal(t, src.Get(t), *ptr.Get(t).(*any))
			})
		})
	}

	andPtrPointsToSomethingWithTheSameType := func(s *testcase.Spec, ptrValue func() interface{}) {
		s.And(`ptr is pointing to the same type`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return ptrValue()
			})

			s.Then(`ptr pointed value equal with source value`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				assert.Equal(t, src.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})
	}

	s.When(`to be linked value is`, func(s *testcase.Spec) {
		s.Context(`a primitive non pointer type`, func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) interface{} {
				return `Hello, World!`
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				var s string
				return &s
			})
		})

		type T struct{ str string }

		s.Context(`a struct type`, func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) interface{} {
				return T{str: RandomName}
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				return &T{}
			})
		})

		s.Context(`a pointer to a struct type`, func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) interface{} {
				return &T{str: RandomName}
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				value := &T{}
				return &value
			})
		})
	})
}

func TestSymbolicName(t *testing.T) {
	subject := reflectkit.SymbolicName

	SpecForPrimitiveNames(t, subject)

	t.Run("when given struct is from different package than the current one", func(t *testing.T) {
		o := SampleType{}
		assert.Equal(t, `reflectkit_test.SampleType`, reflectkit.SymbolicName(o))
	})

	t.Run("when given object is an interface", func(t *testing.T) {
		var i InterfaceObject = &StructObject{}

		assert.Equal(t, `*reflectkit_test.StructObject`, subject(i))
	})

	t.Run("when given object is a struct", func(t *testing.T) {
		assert.Equal(t, `reflectkit_test.StructObject`, subject(StructObject{}))
	})

	t.Run("when given object is a pointer of a struct", func(t *testing.T) {
		assert.Equal(t, `*reflectkit_test.StructObject`, subject(&StructObject{}))
	})

	t.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
		o := &StructObject{}

		assert.Equal(t, `**reflectkit_test.StructObject`, subject(&o))
	})

	t.Run("when the given object is a reflect type", func(t *testing.T) {
		assert.Equal(t, `reflectkit_test.StructObject`, subject(reflectkit.TypeOf[StructObject]()))
		assert.Equal(t, `*reflectkit_test.StructObject`, subject(reflectkit.TypeOf[*StructObject]()))
		assert.Equal(t, `**reflectkit_test.StructObject`, subject(reflectkit.TypeOf[**StructObject]()))
	})

	t.Run("when the given object is a reflect value", func(t *testing.T) {
		assert.Equal(t, `reflectkit_test.StructObject`, subject(reflect.ValueOf(StructObject{})))
		assert.Equal(t, `*reflectkit_test.StructObject`, subject(reflect.ValueOf(&StructObject{})))
	})
}

func TestSetValue(t *testing.T) {
	t.Run("Set a value that can be set", func(t *testing.T) {
		var str string
		rv := reflect.ValueOf(&str)
		reflectkit.SetValue(rv.Elem(), reflect.ValueOf("42"))
		assert.Equal(t, "42", str)
	})
	t.Run("Set an unexported field's value", func(t *testing.T) {
		type V struct{ unexported string }
		var v V
		rv := reflect.ValueOf(&v)
		reflectkit.SetValue(rv.Elem().FieldByName("unexported"), reflect.ValueOf("42"))
		assert.Equal(t, "42", v.unexported)
	})
	t.Run("Set an unexported field's with any type value", func(t *testing.T) {
		type V struct{ unexported any }
		var v V
		rv := reflect.ValueOf(&v)
		reflectkit.SetValue(rv.Elem().FieldByName("unexported"), reflect.ValueOf("42"))
		assert.Equal[any](t, "42", v.unexported)
	})
}

func TestDerefType(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		typ, depth := reflectkit.DerefType(nil)
		assert.Nil(t, typ)
		assert.Equal(t, depth, 0)
	})
	t.Run("base type", func(t *testing.T) {
		typ, depth := reflectkit.DerefType(reflect.TypeOf(""))
		assert.Equal(t, typ, reflect.TypeOf(""))
		assert.Equal(t, depth, 0)
	})
	t.Run("pointer to a type", func(t *testing.T) {
		typ, depth := reflectkit.DerefType(reflect.TypeOf(pointer.Of[string]("")))
		assert.Equal(t, typ, reflect.TypeOf(""))
		assert.Equal(t, depth, 1)
	})
	t.Run("pointer to struct", func(t *testing.T) {
		type X struct{}
		typ, depth := reflectkit.DerefType(reflect.TypeOf(pointer.Of(pointer.Of(X{}))))
		assert.Equal(t, reflectkit.TypeOf[X](), typ)
		assert.Equal(t, depth, 2)
	})
	t.Run("interface to struct", func(t *testing.T) {
		type I interface{}
		type X struct{}

		intType := reflectkit.TypeOf[I]()
		intVal := reflect.New(intType).Elem()
		val := reflect.ValueOf(X{})
		intVal.Set(val)

		got, depth := reflectkit.DerefType(intVal.Type())

		assert.Equal(t, reflectkit.TypeOf[I](), got)
		assert.Equal(t, depth, 0)
	})
}

func TestPointerOf(t *testing.T) {
	t.Run("invalid value", func(t *testing.T) {
		ptr := reflectkit.PointerOf(reflect.ValueOf(nil))
		assert.Equal(t, ptr, reflect.ValueOf(nil))
	})
	t.Run("valid value", func(t *testing.T) {
		v := reflect.ValueOf(rnd.String())
		ptr := reflectkit.PointerOf(v)
		assert.Equal(t, reflect.Pointer, ptr.Kind())
		assert.Equal(t, v, ptr.Elem())
	})
	t.Run("addressable", func(t *testing.T) {
		type T struct {
			ID string
		}
		expFieldValue := rnd.UUID()
		var v T
		ptrVal := reflect.ValueOf(&v)
		ptrID := reflectkit.PointerOf(ptrVal.Elem().FieldByName("ID"))
		ptrID.Elem().Set(reflect.ValueOf(expFieldValue))
		assert.Equal(t, expFieldValue, ptrVal.Elem().FieldByName("ID").Interface().(string))
		assert.Equal(t, expFieldValue, v.ID)
	})
}

func TestToValue(t *testing.T) {
	t.Run("AlreadyReflectValue", func(t *testing.T) {
		expectedRV := reflect.ValueOf(123)
		result := reflectkit.ToValue(expectedRV)
		assert.Equal(t, result, expectedRV)
	})

	t.Run("NonReflectValueType", func(t *testing.T) {
		value := 456
		expectedType := reflect.TypeOf(value)
		result := reflectkit.ToValue(value)
		assert.Equal(t, result.Type(), expectedType)
	})

	t.Run("NilValue", func(t *testing.T) {
		result := reflectkit.ToValue(nil)
		assert.False(t, result.IsValid())
	})

	t.Run("StructType", func(t *testing.T) {
		type exampleStruct struct{ foo string }
		value := exampleStruct{"bar"}
		expectedType := reflect.TypeOf(value)
		result := reflectkit.ToValue(value)
		assert.Equal(t, result.Type(), expectedType)
		assert.Equal(t, result.Interface().(exampleStruct), value)
	})
}

func TestErrTypeMismatch(t *testing.T) {
	assert.Error(t, reflectkit.ErrTypeMismatch)
	assert.NotPanic(t, func() {
		assert.NotEmpty(t, reflectkit.ErrTypeMismatch.Error())
	})
}

type TestStruct2 struct {
	PublicField  string
	privateField string
}

func TestLookupField(t *testing.T) {
	t.Run("when  struct is not a struct", func(t *testing.T) {
		intV := reflect.ValueOf(123)
		field, value, ok := reflectkit.LookupField(intV, "field")
		if field.Name != "" || value.IsValid() || ok {
			t.Errorf("expected field to be empty and value to be zero value when input is not a struct, but got %+v, %+v", field, value)
		}
	})

	t.Run("when  struct is invalid", func(t *testing.T) {
		intV := reflect.Value{}
		field, value, ok := reflectkit.LookupField(intV, "field")
		if field.Name != "" || value.IsValid() || ok {
			t.Errorf("expected field to be empty and value to be zero value when input is invalid, but got %+v, %+v", field, value)
		}
	})

	t.Run("when  struct has no such field", func(t *testing.T) {
		structV := reflect.ValueOf(TestStruct2{})
		field, value, ok := reflectkit.LookupField(structV, "nonExistent")
		if field.Name != "" || value.IsValid() || ok {
			t.Errorf("expected field to be empty and value to be zero value when struct has no such field, but got %+v, %+v", field, value)
		}
	})

	t.Run("when  struct has a public field", func(t *testing.T) {
		structV := reflect.ValueOf(TestStruct2{PublicField: "public"})
		field, value, ok := reflectkit.LookupField(structV, "PublicField")
		if !ok || field.Name != "PublicField" || !value.IsValid() || value.String() != "public" {
			t.Errorf("expected to get public field and its value when struct has a public field, but got %+v, %+v", field, value)
		}
	})

	t.Run("when  struct has an unexported field", func(t *testing.T) {
		structV := reflect.ValueOf(TestStruct2{privateField: "private"})
		field, value, ok := reflectkit.LookupField(structV, "privateField")
		if !ok || field.Name != "privateField" || !value.IsValid() {
			t.Errorf("expected to get unexported field and its zero value when struct has an unexported field, but got %+v, %+v", field, value)
		}
	})
}

func TestLookupFieldWithNilStruct(t *testing.T) {
	var nilV *struct{}
	structV := reflect.ValueOf(nilV).Elem()
	field, value, ok := reflectkit.LookupField(structV, "field")
	if field.Name != "" || value.IsValid() || ok {
		t.Errorf("expected field to be empty and value to be zero value when input is a nil struct, but got %+v, %+v", field, value)
	}
}

func TestLookupFieldWithNonStructValue(t *testing.T) {
	structV := reflect.ValueOf(reflect.StructField{})
	field, value, ok := reflectkit.LookupField(structV, "field")
	if field.Name != "" || value.IsValid() || ok {
		t.Errorf("expected field to be empty and value to be zero value when input is not a struct value, but got %+v, %+v", field, value)
	}
}

func TestLookupFieldWithNonStringName(t *testing.T) {
	structV := reflect.ValueOf(TestStruct2{})
	field, value, ok := reflectkit.LookupField(structV, "")
	if field.Name != "" || value.IsValid() || ok {
		t.Errorf("expected field to be empty and value to be zero value when input name is not a string, but got %+v, %+v", field, value)
	}
}

func TestLookupFieldWithUnreachableValue(t *testing.T) {
	structV := reflect.ValueOf(TestStruct2{})
	field, value, ok := reflectkit.LookupField(structV, "unreachable")
	assert.False(t, ok)
	assert.Empty(t, field.Name)
	assert.False(t, value.IsValid())
}

func ExampleToSettable() {
	type T struct{ v int }
	var v T

	ptr := reflect.ValueOf(&v)
	rStruct := ptr.Elem()

	got, ok := reflectkit.ToSettable(rStruct.FieldByName("v"))
	_, _ = got, ok
}

func TestToSettable(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		_, ok := reflectkit.ToSettable(reflect.Value{})
		assert.False(t, ok)
	})
	t.Run("nil", func(t *testing.T) {
		_, ok := reflectkit.ToSettable(reflect.ValueOf((*string)(nil)))
		assert.False(t, ok)
	})
	t.Run("unaddressable", func(t *testing.T) {
		v := reflect.ValueOf(42)

		_, ok := reflectkit.ToSettable(v)
		assert.False(t, ok)
	})
	t.Run("addressable", func(t *testing.T) {
		var n = 42
		ptr := reflect.ValueOf(&n)
		elem := ptr.Elem()

		got, ok := reflectkit.ToSettable(elem)
		assert.True(t, ok)
		assert.Equal(t, elem, got)

		got.Set(reflect.ValueOf(24))

		assert.Equal(t, n, 24)
	})
	t.Run("addressable but unexported struct field", func(t *testing.T) {
		type T struct{ v int }
		var v T
		ptr := reflect.ValueOf(&v)
		rStruct := ptr.Elem()

		field := rStruct.FieldByName("v")
		assert.False(t, field.CanSet())

		got, ok := reflectkit.ToSettable(field)
		assert.True(t, ok)
		got.Set(reflect.ValueOf(42))

		assert.Equal(t, v.v, 42)
	})
}
