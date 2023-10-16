package reflectkit_test

import (
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"reflect"
	"testing"
)

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

	assert.Must(t).Equal(expectedValueType, subject(plainStruct))
	assert.Must(t).Equal(expectedValueType, subject(ptrToStruct))
	assert.Must(t).Equal(expectedValueType, subject(ptrToPtr))
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

	assert.Must(t).Equal(expectedValueType, subject(plainStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToPtr).Type())
}

func TestBaseValue(t *testing.T) {
	subject := func(input interface{}) reflect.Value {
		return reflectkit.BaseValue(reflect.ValueOf(input))
	}

	SpecForPrimitiveNames(t, func(obj interface{}) string {
		return subject(obj).Type().Name()
	})

	expectedValue := reflect.ValueOf(StructObject{})
	expectedValueType := expectedValue.Type()

	plainStruct := StructObject{}
	ptrToStruct := &plainStruct
	ptrToPtr := &ptrToStruct

	assert.Must(t).Equal(expectedValueType, subject(plainStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToStruct).Type())
	assert.Must(t).Equal(expectedValueType, subject(ptrToPtr).Type())

	invalid := reflect.Value{}
	assert.Equal(t, invalid, reflectkit.BaseValue(invalid))
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

func TestFullyQualifiedName(t *testing.T) {
	t.Run("FullyQualifiedName", func(spec *testing.T) {

		subject := reflectkit.FullyQualifiedName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()

			o := crudcontracts.Creator[int, string](nil)

			assert.Must(t).Equal(`"go.llib.dev/frameless/ports/crud/crudcontracts".Creator[int,string]`, subject(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			assert.Must(t).Equal(`"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			assert.Must(t).Equal(`"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, subject(&o))
		})

	})

}

func SpecForPrimitiveNames(spec *testing.T, subject func(entity interface{}) string) {

	spec.Run("when given object is a bool", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("bool", subject(true))
	})

	spec.Run("when given object is a string", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("string", subject(`42`))
	})

	spec.Run("when given object is a int", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int", subject(int(42)))
	})

	spec.Run("when given object is a int8", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int8", subject(int8(42)))
	})

	spec.Run("when given object is a int16", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int16", subject(int16(42)))
	})

	spec.Run("when given object is a int32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int32", subject(int32(42)))
	})

	spec.Run("when given object is a int64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("int64", subject(int64(42)))
	})

	spec.Run("when given object is a uintptr", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uintptr", subject(uintptr(42)))
	})

	spec.Run("when given object is a uint", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint", subject(uint(42)))
	})

	spec.Run("when given object is a uint8", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint8", subject(uint8(42)))
	})

	spec.Run("when given object is a uint16", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint16", subject(uint16(42)))
	})

	spec.Run("when given object is a uint32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint32", subject(uint32(42)))
	})

	spec.Run("when given object is a uint64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("uint64", subject(uint64(42)))
	})

	spec.Run("when given object is a float32", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("float32", subject(float32(42)))
	})

	spec.Run("when given object is a float64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("float64", subject(float64(42)))
	})

	spec.Run("when given object is a complex64", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("complex64", subject(complex64(42)))
	})

	spec.Run("when given object is a complex128", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Equal("complex128", subject(complex128(42)))
	})

}

func TestIsValueEmpty(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Var[any]{ID: `input value`}
	subject := func(t *testcase.T) bool {
		return reflectkit.IsValueEmpty(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an pointer to an zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := ""
			return &v
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
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
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an empty slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
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
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an empty map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
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
			assert.Must(t).True(subject(t))
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
			assert.Must(t).True(subject(t))
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

func TestIsValueNil(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Let[any](s, nil)
	act := func(t *testcase.T) bool {
		return reflectkit.IsValueNil(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as nil`, func(t *testcase.T) {
			assert.Must(t).True(act(t))
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
			assert.Must(t).True(act(t))
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
			assert.Must(t).True(act(t))
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
			assert.Must(t).True(act(t))
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
			assert.Must(t).True(act(t))
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
				assert.Must(t).Equal(src.Get(t), *ptr.Get(t).(*any))
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

				assert.Must(t).Equal(src.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
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

func TestName(t *testing.T) {
	t.Run("SymbolicName", func(spec *testing.T) {

		subject := reflectkit.SymbolicName

		SpecForPrimitiveNames(t, subject)

		spec.Run("when given struct is from different package than the current one", func(t *testing.T) {
			t.Parallel()
			o := crudcontracts.Creator[int, string](nil)
			assert.Must(t).Equal(`crudcontracts.Creator[int,string]`, reflectkit.SymbolicName(o))
		})

		spec.Run("when given object is an interface", func(t *testing.T) {
			t.Parallel()

			var i InterfaceObject = &StructObject{}

			assert.Must(t).Equal(`reflectkit_test.StructObject`, subject(i))
		})

		spec.Run("when given object is a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`reflectkit_test.StructObject`, subject(StructObject{}))
		})

		spec.Run("when given object is a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			assert.Must(t).Equal(`reflectkit_test.StructObject`, subject(&StructObject{}))
		})

		spec.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
			t.Parallel()

			o := &StructObject{}

			assert.Must(t).Equal(`reflectkit_test.StructObject`, subject(&o))
		})

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
