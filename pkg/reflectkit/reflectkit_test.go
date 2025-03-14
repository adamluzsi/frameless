package reflectkit_test

import (
	"context"
	"fmt"
	"iter"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
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
	act := reflectkit.FullyQualifiedName

	SpecForPrimitiveNames(t, act)

	t.Run("when given struct is from different package than the current one", func(t *testing.T) {
		o := SampleType{}

		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".SampleType`, act(o))
	})

	t.Run("when given object is an interface", func(t *testing.T) {
		var i InterfaceObject = &StructObject{}

		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(i))
	})

	t.Run("when given object is a struct", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(StructObject{}))
	})

	t.Run("when given object is a pointer of a struct", func(t *testing.T) {
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(&StructObject{}))
	})

	t.Run("when given object is a pointer of a pointer of a struct", func(t *testing.T) {
		o := &StructObject{}
		assert.Equal(t, `**"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(&o))
	})

	t.Run("when the given object is a reflect type", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(reflectkit.TypeOf[StructObject]()))
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(reflectkit.TypeOf[*StructObject]()))
		assert.Equal(t, `**"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(reflectkit.TypeOf[**StructObject]()))
	})

	t.Run("when the given object is a reflect value", func(t *testing.T) {
		assert.Equal(t, `"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(reflect.ValueOf(StructObject{})))
		assert.Equal(t, `*"go.llib.dev/frameless/pkg/reflectkit_test".StructObject`, act(reflect.ValueOf(&StructObject{})))
	})

	t.Run("non-defined types", func(t *testing.T) {
		assert.Should(t).Equal(reflectkit.FullyQualifiedName(reflectkit.TypeOf[map[string]int]()), "map[string]int")
		assert.Should(t).Equal(reflectkit.FullyQualifiedName(reflectkit.TypeOf[[]int]()), "[]int")
		assert.Should(t).Equal(strings.ReplaceAll(reflectkit.FullyQualifiedName(reflectkit.TypeOf[struct{}]()), " ", ""), "struct{}")
		assert.Should(t).Equal(reflectkit.FullyQualifiedName(reflectkit.TypeOf[int]()), "int")
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
	_ = v.v

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

func TestTagHandler_smoke(t *testing.T) {
	type T struct {
		V string `default:"foo"`
	}

	var DefaultTag = reflectkit.TagHandler[reflect.Value]{
		Name: "default",
		Parse: func(sf reflect.StructField, tag string) (reflect.Value, error) {
			return convkit.ParseReflect(sf.Type, tag)
		},
		Use: func(sf reflect.StructField, field, defaultValue reflect.Value) error {
			if field.IsZero() {
				field.Set(defaultValue) // defaultValue is the result of Parse
			}
			return nil
		},
	}

	v1 := T{}
	assert.NoError(t, DefaultTag.HandleStruct(reflect.ValueOf(&v1).Elem()))
	assert.Equal(t, v1.V, "foo")

	v2 := T{V: "bar"}
	assert.NoError(t, DefaultTag.HandleStruct(reflect.ValueOf(&v1).Elem()))
	assert.Equal(t, v2.V, "bar")
}

func TestTagHandler_panicOnParseError(t *testing.T) {
	var parseErr error
	var useErr error

	var tagH = reflectkit.TagHandler[any]{
		Name: "mytag",
		Parse: func(sf reflect.StructField, tagValue string) (any, error) {
			return nil, parseErr
		},
		Use: func(sf reflect.StructField, field reflect.Value, v any) error {
			return useErr
		},

		PanicOnParseError: true,
	}

	type T struct {
		V string `mytag:"testing"`
	}

	rStruct := reflect.ValueOf(T{})
	field, value, _ := reflectkit.LookupField(rStruct, "V")

	useErr = rnd.Error()
	assert.ErrorIs(t, tagH.HandleStruct(rStruct), useErr)
	assert.ErrorIs(t, tagH.HandleStructField(field, value), useErr)

	useErr = nil
	assert.NoError(t, tagH.HandleStruct(rStruct))
	assert.NoError(t, tagH.HandleStructField(field, value))

	parseErr = rnd.Error()
	got := assert.Panic(t, func() { tagH.HandleStruct(rStruct) })
	assert.Equal[any](t, got, parseErr)
	got = assert.Panic(t, func() { tagH.HandleStructField(field, value) })
	assert.Equal[any](t, got, parseErr)
}

func TestTagHandler_HandleStruct(t *testing.T) {
	t.Run("non_struct_type", func(t *testing.T) {
		type NotStruct int
		var ns NotStruct = 42

		handler := reflectkit.TagHandler[any]{ // Initialize handler with dummy functions for testing
			Name:  "test",
			Parse: func(sf reflect.StructField, tag string) (any, error) { return nil, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v any) error { return nil },
		}

		assert.ErrorIs(t, handler.HandleStruct(reflect.ValueOf(ns)), interr.ImplementationError)
	})

	t.Run("invalid_reflect_value", func(t *testing.T) {
		handler := reflectkit.TagHandler[any]{ // Initialize handler with dummy functions for testing
			Name:  "test",
			Parse: func(sf reflect.StructField, tag string) (any, error) { return nil, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v any) error { return nil },
		}

		assert.ErrorIs(t, handler.HandleStruct(reflect.ValueOf(nil)), interr.ImplementationError)
	})

	t.Run("presence_of_specified_tag", func(t *testing.T) {
		type TestStruct struct {
			Field1 string `testtag:"value1"`
			Field2 int    `testtag:"42"`
		}

		var vs []string

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				vs = append(vs, v)
				return nil
			},
		}

		ts := TestStruct{}
		assert.NoError(t, handler.HandleStruct(reflect.ValueOf(ts)))
		assert.ContainExactly(t, vs, []string{"value1", "42"})
	})

	t.Run("absence_of_specified_tag", func(t *testing.T) {
		type TestStruct struct {
			Field1 string `othertag:"value"`
			Field2 int
		}

		var n int
		handler := reflectkit.TagHandler[any]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (any, error) {
				n++
				return nil, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v any) error {
				n++
				return nil
			},
		}

		ts := TestStruct{Field1: "test", Field2: 42}
		assert.NoError(t, handler.HandleStruct(reflect.ValueOf(ts)))
		assert.Equal(t, 0, n, "expected that neither Parse, nor Use is called")
	})

	t.Run("working with addressable field", func(t *testing.T) {
		type TestStruct struct {
			StringField string `testtag:"hello"`
		}

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				field.SetString(v)
				return nil
			},
		}

		ts := TestStruct{}
		assert.NoError(t, handler.HandleStruct(reflect.ValueOf(&ts).Elem()))
		assert.Equal(t, ts.StringField, "hello")
	})

	t.Run("parse error propagated back", func(t *testing.T) {
		type TestStruct struct {
			StringField string `testtag:"hello"`
		}

		var expErr = rnd.Error()
		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, expErr },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		ts := TestStruct{}
		assert.ErrorIs(t, handler.HandleStruct(reflect.ValueOf(ts)), expErr)
	})

	t.Run("use error propagated back", func(t *testing.T) {
		type TestStruct struct {
			StringField string `testtag:"hello"`
		}

		var expErr = rnd.Error()
		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return expErr },
		}

		ts := TestStruct{}
		assert.ErrorIs(t, handler.HandleStruct(reflect.ValueOf(ts)), expErr)
	})

	t.Run("HandleUntagged", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct{ V string }

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				parseCount++
				assert.Empty(t, tag)
				return "foo", nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				useCount++
				assert.Equal(t, "foo", v)
				return nil
			},
			HandleUntagged: true,
		}

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStruct(reflect.ValueOf(T{})))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, 1)
		assert.Equal(t, useCount, n, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("parse caching", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type TestStruct struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				parseCount++
				return tag, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				useCount++
				return nil
			},
		}

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStruct(reflect.ValueOf(TestStruct{})))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, 2, "two, because each field's tag is parsed once")
		assert.Equal(t, useCount, n*2, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("by default no caching for mutable tag value types", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[*string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*string, error) {
				parseCount++
				return &tag, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v *string) error {
				useCount++
				return nil
			},
		}

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStruct(reflect.ValueOf(T{})))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, n*2, "two, because each field's tag is parsed once")
		assert.Equal(t, useCount, n*2, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("forced caching for mutable tag value types", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				parseCount++
				return tag, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				useCount++
				return nil
			},

			ForceCache: true,
		}

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStruct(reflect.ValueOf(T{})))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, 2, "two, because each field's tag is parsed once")
		assert.Equal(t, useCount, n*2, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("cache for mutable types", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type TestStruct struct {
			Field1 string `testtag:"value"`
			Field2 int    `testtag:"value"`
		}

		type S struct {
			V int
		}

		handler := reflectkit.TagHandler[*S]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*S, error) {
				parseCount++
				return &S{}, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v *S) error {
				useCount++
				return nil
			},
		}

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStruct(reflect.ValueOf(TestStruct{})))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, n*2, "because each field's tag is parsed once")
		assert.Equal(t, useCount, n*2, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("race", func(t *testing.T) {
		type T struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		testcase.Race(func() {
			handler.HandleStruct(reflect.ValueOf(T{}))
		}, func() {
			handler.HandleStruct(reflect.ValueOf(T{}))
		})
	})
}

func TestTagHandler_HandleStructField(t *testing.T) {
	t.Run("invalid struct field", func(t *testing.T) {
		handler := reflectkit.TagHandler[any]{ // Initialize handler with dummy functions for testing
			Name:  "test",
			Parse: func(sf reflect.StructField, tag string) (any, error) { return nil, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v any) error { return nil },
		}

		type T struct{ V int }

		_, field, ok := reflectkit.LookupField(reflect.ValueOf(T{}), "V")
		assert.True(t, ok)
		assert.ErrorIs(t, handler.HandleStructField(reflect.StructField{}, field), interr.ImplementationError)
	})

	t.Run("invalid field value", func(t *testing.T) {
		handler := reflectkit.TagHandler[any]{ // Initialize handler with dummy functions for testing
			Name:  "test",
			Parse: func(sf reflect.StructField, tag string) (any, error) { return nil, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v any) error { return nil },
		}

		type T struct{ V int }

		sf, _, ok := reflectkit.LookupField(reflect.ValueOf(T{}), "V")
		assert.True(t, ok)
		assert.ErrorIs(t, handler.HandleStructField(sf, reflect.Value{}), interr.ImplementationError)
	})

	t.Run("presence_of_specified_tag", func(t *testing.T) {
		type T struct {
			Field1 string `testtag:"value1"`
			Field2 int    `testtag:"42"`
		}

		var vs []string

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				vs = append(vs, v)
				return nil
			},
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "Field1")
		assert.True(t, ok)
		assert.NoError(t, handler.HandleStructField(sf, field))
		assert.ContainExactly(t, vs, []string{"value1"})

		sf, field, ok = reflectkit.LookupField(reflect.ValueOf(v), "Field2")
		assert.True(t, ok)
		assert.NoError(t, handler.HandleStructField(sf, field))

		assert.ContainExactly(t, vs, []string{"value1", "42"})
	})

	t.Run("absence_of_specified_tag", func(t *testing.T) {
		type T struct {
			Field1 string `othertag:"value"`
		}

		var n int
		handler := reflectkit.TagHandler[any]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (any, error) {
				n++
				return nil, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v any) error {
				n++
				return nil
			},
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "Field1")
		assert.True(t, ok)
		assert.NoError(t, handler.HandleStructField(sf, field))

		assert.Equal(t, 0, n, "expected that neither Parse, nor Use is called")
	})

	t.Run("working with addressable field", func(t *testing.T) {
		type T struct {
			StringField string `testtag:"hello"`
		}

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				field.SetString(v)
				return nil
			},
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(&v).Elem(), "StringField")
		assert.True(t, ok)
		assert.NoError(t, handler.HandleStructField(sf, field))

		assert.Equal(t, v.StringField, "hello")
	})

	t.Run("parse error propagated back", func(t *testing.T) {
		type T struct {
			StringField string `testtag:"hello"`
		}

		var expErr = rnd.Error()
		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, expErr },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "StringField")
		assert.True(t, ok)
		assert.ErrorIs(t, handler.HandleStructField(sf, field), expErr)
	})

	t.Run("use error propagated back", func(t *testing.T) {
		type T struct {
			StringField string `testtag:"hello"`
		}

		var expErr = rnd.Error()
		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return expErr },
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "StringField")
		assert.True(t, ok)
		assert.ErrorIs(t, handler.HandleStructField(sf, field), expErr)
	})

	t.Run("cache for immutable types", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct {
			V string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				parseCount++
				return tag, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				useCount++
				return nil
			},
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStructField(sf, field))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, 1, "tag should have been parsed only once")
		assert.Equal(t, useCount, n, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("cache for mutable types", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct {
			V string `testtag:"value"`
		}

		type S struct {
			V int
		}

		handler := reflectkit.TagHandler[*S]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*S, error) {
				parseCount++
				return &S{}, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v *S) error {
				useCount++
				return nil
			},
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStructField(sf, field))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, n, "because field should be parsed on each run to make it deterministic if Use mutates the value")
		assert.Equal(t, useCount, n, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("cache for mutable types when ForceCache enabled", func(t *testing.T) {
		var (
			parseCount int
			useCount   int
		)

		type T struct {
			V string `testtag:"value"`
		}

		type S struct{ V int }

		handler := reflectkit.TagHandler[*S]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*S, error) {
				parseCount++
				return &S{}, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v *S) error {
				useCount++
				return nil
			},

			ForceCache: true,
		}

		v := T{}
		sf, field, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			assert.NoError(t, handler.HandleStructField(sf, field))
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })

		assert.Equal(t, parseCount, 1)
		assert.Equal(t, useCount, n, "twice of the repeat count, because we have two field in the struct")
	})

	t.Run("race", func(t *testing.T) {
		type T struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		rStruct := reflect.ValueOf(T{})
		field1, val1, ok := reflectkit.LookupField(rStruct, "Field1")
		assert.True(t, ok)
		field2, val2, ok := reflectkit.LookupField(rStruct, "Field2")
		assert.True(t, ok)

		testcase.Race(
			func() { handler.HandleStructField(field1, val1) },
			func() { handler.HandleStructField(field1, val1) },
			func() { handler.HandleStructField(field2, val2) },
			func() { handler.HandleStructField(field2, val2) },
		)
	})
}

func TestTagHandler_alias(t *testing.T) {
	type T struct {
		V1 string `testtag:"x"`
		V2 string `tt1:"y"`
		V3 string `tt2:"z"`
	}

	var found []string
	th := reflectkit.TagHandler[string]{
		Name:  "testtag",
		Alias: []string{"tt1", "tt2"},

		Parse: func(field reflect.StructField, tagValue string) (string, error) {
			return tagValue, nil
		},

		Use: func(field reflect.StructField, value reflect.Value, v string) error {
			found = append(found, v)
			return nil
		},
	}

	var v T

	rStruct := reflect.ValueOf(v)
	assert.NoError(t, th.HandleStruct(rStruct))
	assert.ContainExactly(t, []string{"x", "y", "z"}, found)
	found = nil

	field, value, ok := reflectkit.LookupField(rStruct, "V2")
	assert.True(t, ok)
	assert.NoError(t, th.HandleStructField(field, value))
	assert.ContainExactly(t, []string{"y"}, found)
}

func TestTagHandler_LookupTag(t *testing.T) {
	t.Run("presence_of_specified_tag", func(t *testing.T) {
		type T struct {
			Field1 string `testtag:"value1"`
			Field2 int    `testtag:"42"`
		}

		var vs []string

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				vs = append(vs, tag)
				return tag, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		v := T{}
		field, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "Field1")
		assert.True(t, ok)
		tag, ok, err := handler.LookupTag(field)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, tag, "value1")

		field, _, ok = reflectkit.LookupField(reflect.ValueOf(v), "Field2")
		assert.True(t, ok)
		tag, ok, err = handler.LookupTag(field)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, tag, "42")

		assert.ContainExactly(t, []string{"value1", "42"}, vs)
	})

	t.Run("absence of specified tag", func(t *testing.T) {
		type T struct {
			Field1 string `othertag:"value"`
		}

		var n int
		handler := reflectkit.TagHandler[any]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (any, error) {
				n++
				return nil, nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v any) error {
				n++
				return nil
			},
		}

		v := T{}
		field, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "Field1")
		assert.True(t, ok)
		_, ok, err := handler.LookupTag(field)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("absence of specified tag but with untagged handling", func(t *testing.T) {
		type T struct {
			Field1 string `othertag:"value"`
		}

		var n int
		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				n++
				return "foo", nil
			},
			Use: func(sf reflect.StructField, field reflect.Value, v string) error {
				n++
				return nil
			},
			HandleUntagged: true,
		}

		v := T{}
		field, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "Field1")
		assert.True(t, ok)
		tag, ok, err := handler.LookupTag(field)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, tag, "foo")
	})

	t.Run("parse error propagated back", func(t *testing.T) {
		type T struct {
			StringField string `testtag:"hello"`
		}

		var expErr = rnd.Error()
		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, expErr },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		v := T{}
		sf, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "StringField")
		assert.True(t, ok)
		_, _, err := handler.LookupTag(sf)
		assert.ErrorIs(t, err, expErr)
	})

	t.Run("cache for immutable types", func(t *testing.T) {
		var parseCount int

		type T struct {
			V string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) {
				parseCount++
				return tag, nil
			},
		}

		v := T{}
		sf, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			_, _, err := handler.LookupTag(sf)
			assert.NoError(t, err)
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })
		assert.Equal(t, parseCount, 1, "tag should have been parsed only once")
	})

	t.Run("cache for mutable types", func(t *testing.T) {
		var parseCount int

		type T struct {
			V string `testtag:"value"`
		}

		type S struct {
			V int
		}

		handler := reflectkit.TagHandler[*S]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*S, error) {
				parseCount++
				return &S{}, nil
			},
		}

		v := T{}
		sf, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			_, _, err := handler.LookupTag(sf)
			assert.NoError(t, err)
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })
		assert.Equal(t, parseCount, n, "because field should be parsed on each run to make it deterministic if Use mutates the value")
	})

	t.Run("cache for mutable types when ForceCache enabled", func(t *testing.T) {
		var parseCount int

		type T struct {
			V string `testtag:"value"`
		}

		type S struct{ V int }

		handler := reflectkit.TagHandler[*S]{
			Name: "testtag",
			Parse: func(sf reflect.StructField, tag string) (*S, error) {
				parseCount++
				return &S{}, nil
			},
			ForceCache: true,
		}

		v := T{}
		sf, _, ok := reflectkit.LookupField(reflect.ValueOf(v), "V")
		assert.True(t, ok)

		n := rnd.Repeat(3, 7, func() {
			_, _, err := handler.LookupTag(sf)
			assert.NoError(t, err)
		})

		testcase.OnFail(t, func() { t.Log("repeat count:", n) })
		assert.Equal(t, parseCount, 1)
	})

	t.Run("race", func(t *testing.T) {
		type T struct {
			Field1 string `testtag:"value"`
			Field2 string `testtag:"value"`
		}

		handler := reflectkit.TagHandler[string]{
			Name:  "testtag",
			Parse: func(sf reflect.StructField, tag string) (string, error) { return tag, nil },
			Use:   func(sf reflect.StructField, field reflect.Value, v string) error { return nil },
		}

		rStruct := reflect.ValueOf(T{})
		field1, _, ok := reflectkit.LookupField(rStruct, "Field1")
		assert.True(t, ok)
		field2, _, ok := reflectkit.LookupField(rStruct, "Field2")
		assert.True(t, ok)

		testcase.Race(
			func() { handler.LookupTag(field1) },
			func() { handler.LookupTag(field1) },
			func() { handler.LookupTag(field2) },
			func() { handler.LookupTag(field2) },
		)
	})
}

func TestClone(t *testing.T) {
	t.Run("Clone nil value", func(t *testing.T) {
		var nilVal reflect.Value
		cloned := reflectkit.Clone(nilVal)
		assert.False(t, cloned.IsValid())
	})

	t.Run("Clone integer", func(t *testing.T) {
		{
			val := reflect.ValueOf(int(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int](t, 42, int(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int8(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int8](t, 42, int8(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int16(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int16](t, 42, int16(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int32(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int32](t, 42, int32(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int64(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int64](t, 42, int64(cloned.Int()))
		}
	})

	t.Run("Clone struct", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		val := reflect.ValueOf(sample{A: 10, B: "test"})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.FieldByName("B").Set(reflect.ValueOf("foo"))
		assert.Equal(t, val.FieldByName("B").String(), "test")
	})

	t.Run("Clone slice and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf([]int{1, 2, 3})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.Index(0).SetInt(99)
		assert.Equal(t, 1, val.Index(0).Int())
		assert.NotEqual(t, 99, val.Index(0).Int())
	})

	t.Run("Clone array and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf([3]int{1, 2, 3})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		assert.Equal(t, 1, val.Index(0).Int())
		assert.NotEqual(t, 99, val.Index(0).Int())
	})

	t.Run("Clone map and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf(map[string]int{"a": 1, "b": 2})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.SetMapIndex(reflect.ValueOf("a"), reflect.ValueOf(99))
		assert.NotEqual(t, 99, val.MapIndex(reflect.ValueOf("a")).Int())
	})

	t.Run("Clone chan and mutate copy", func(t *testing.T) {
		og := reflect.ValueOf(make(chan int))
		defer og.Close()
		cloned := reflectkit.Clone(og)
		assert.False(t, reflectkit.IsNil(cloned))
		defer cloned.Close()

		var ogRec, clRec bool
		go func() {
			_, ok := og.Recv()
			clRec = ok
		}()
		go func() {
			v, ok := cloned.Recv()
			clRec = ok
			assert.Equal(t, int(v.Int()), 42)
		}()

		assert.Within(t, time.Second, func(context.Context) {
			cloned.Send(reflect.ValueOf(int(42)))
		})

		assert.Eventually(t, time.Second, func(t assert.It) {
			assert.True(t, clRec)
			assert.False(t, ogRec)
		})
	})

	t.Run("Cloned chan has the same buffer size", func(t *testing.T) {
		og := reflect.ValueOf(make(chan int, 1))
		defer og.Close()
		cloned := reflectkit.Clone(og)
		assert.False(t, reflectkit.IsNil(cloned))
		defer cloned.Close()

		assert.Within(t, time.Second, func(context.Context) {
			og.Send(reflect.ValueOf(int(42)))
		})

		assert.Within(t, time.Second, func(context.Context) {
			cloned.Send(reflect.ValueOf(int(42)))
		})

		assert.Within(t, time.Second, func(context.Context) {
			val, ok := og.Recv()
			assert.True(t, ok)
			assert.Equal(t, val.Int(), 42)
		})

		assert.Within(t, time.Second, func(context.Context) {
			val, ok := cloned.Recv()
			assert.True(t, ok)
			assert.Equal(t, val.Int(), 42)
		})
	})

	t.Run("Clone struct with nested values", func(t *testing.T) {
		type nested struct {
			X int
		}
		type sample struct {
			A nested
			B string
		}
		val := reflect.ValueOf(sample{A: nested{X: 42}, B: "test"})
		cloned := reflectkit.Clone(val)
		cloned.FieldByName("A").FieldByName("X").SetInt(99)
		assert.NotEqual(t, 99, val.FieldByName("A").FieldByName("X").Int())
	})
}

func TestOverStruct(t *testing.T) {
	type T struct {
		Foo string
		Bar string
		Baz string
	}

	var example = T{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	t.Run("iter.Pull2", func(t *testing.T) {
		i := reflectkit.OverStruct(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			fields []string
			n      int
		)
		for {
			sf, val, ok := next()
			if !ok {
				break
			}
			n++
			fields = append(fields, sf.Name)
			assert.Equal(t, val.String(), strings.ToLower(sf.Name))
		}
		assert.ContainExactly(t, fields, []string{"Foo", "Bar", "Baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("range", func(t *testing.T) {
		var (
			fields []string
			n      int
		)
		for sf, val := range reflectkit.OverStruct(reflect.ValueOf(example)) {
			n++
			fields = append(fields, sf.Name)
			assert.Equal(t, val.String(), strings.ToLower(sf.Name))
		}
		assert.ContainExactly(t, fields, []string{"Foo", "Bar", "Baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not struct kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.OverStruct(reflect.ValueOf("hello:world"))
		})
	})
}

func TestOverMap(t *testing.T) {
	var example = map[string]string{
		"Foo": "foo",
		"Bar": "bar",
		"Baz": "baz",
	}

	t.Run("iter.Pull2 on non empty map", func(t *testing.T) {
		i := reflectkit.OverMap(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			elems []string
			n     int
		)
		for {
			key, val, ok := next()
			if !ok {
				break
			}
			n++
			elems = append(elems, fmt.Sprintf("%s:%s", key.String(), val.String()))
		}
		assert.ContainExactly(t, elems, []string{"Foo:foo", "Bar:bar", "Baz:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("iter.Pull2 on nil map", func(t *testing.T) {
		i := reflectkit.OverMap(reflect.ValueOf((map[string]string)(nil)))

		next, stop := iter.Pull2(i)
		defer stop()

		for {
			_, _, ok := next()
			if !ok {
				break
			}
			t.Fatal("unexpected to have even a single iteration for a nil map")
		}
	})

	t.Run("range on non empty map", func(t *testing.T) {
		var (
			elems []string
			n     int
		)
		for key, val := range reflectkit.OverMap(reflect.ValueOf(example)) {
			n++
			assert.Equal(t, val.String(), strings.ToLower(key.String()))
			elems = append(elems, fmt.Sprintf("%s:%s", key.String(), val.String()))
		}
		assert.ContainExactly(t, elems, []string{"Foo:foo", "Bar:bar", "Baz:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not map kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.OverMap(reflect.ValueOf("hello:world"))
		})
	})
}

func TestOverSlice(t *testing.T) {
	var example = []string{"foo", "bar", "baz"}

	t.Run("iter.Pull2 on non empty slice", func(t *testing.T) {
		i := reflectkit.OverSlice(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			elems []string
			n     int
			last  = -1
		)
		for {
			index, val, ok := next()
			if !ok {
				break
			}
			assert.True(t, last < index)
			last = index
			n++
			elems = append(elems, fmt.Sprintf("%d:%s", index, val.String()))
		}
		assert.ContainExactly(t, elems, []string{"0:foo", "1:bar", "2:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("iter.Pull2 on nil slice", func(t *testing.T) {
		i := reflectkit.OverSlice(reflect.ValueOf(([]string)(nil)))

		next, stop := iter.Pull2(i)
		defer stop()

		for {
			_, _, ok := next()
			if !ok {
				break
			}
			t.Fatal("unexpected to have any value")
		}
	})

	t.Run("range on non empty slice", func(t *testing.T) {
		var (
			elems []string
			n     int
		)
		for index, val := range reflectkit.OverSlice(reflect.ValueOf(example)) {
			n++
			elems = append(elems, fmt.Sprintf("%d:%s", index, val.String()))
		}
		assert.ContainExactly(t, elems, []string{"0:foo", "1:bar", "2:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not map kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.OverSlice(reflect.ValueOf("hello:world"))
		})
	})
}

func TestIsBuiltInType(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[string]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[uint]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[uint8]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[uint16]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[uint32]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[uint64]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[int]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[int8]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[int16]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[int32]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[int64]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[float32]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[float64]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[complex64]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[complex128]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[bool]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[rune]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[byte]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[[]int]()))
		assert.True(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[map[string]int]()))
	})

	t.Run("control", func(t *testing.T) {
		type mystring string
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[mystring]()))
		type myuint uint
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myuint]()))
		type myuint8 uint8
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myuint8]()))
		type myuint16 uint16
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myuint16]()))
		type myuint32 uint32
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myuint32]()))
		type myuint64 uint64
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myuint64]()))
		type myint int
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myint]()))
		type myint8 int8
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myint8]()))
		type myint16 int16
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myint16]()))
		type myint32 int32
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myint32]()))
		type myint64 int64
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myint64]()))
		type myfloat32 float32
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myfloat32]()))
		type myfloat64 float64
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myfloat64]()))
		type mycomplex64 complex64
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[mycomplex64]()))
		type mycomplex128 complex128
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[mycomplex128]()))
		type mybool bool
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[mybool]()))
		type myrune rune
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[myrune]()))
		type mybyte byte
		assert.False(t, reflectkit.IsBuiltInType(reflectkit.TypeOf[mybyte]()))
	})
}

func TestCompare(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		val = let.Var[reflect.Value](s, nil)
		oth = let.Var[reflect.Value](s, nil)
	)
	act := let.Act2(func(t *testcase.T) (int, error) {
		return reflectkit.Compare(val.Get(t), oth.Get(t))
	})

	s.Describe("int", SpecCompare[int](act, val, oth, LessNumber[int], MoreNumber[int]))
	s.Describe("int8", SpecCompare[int8](act, val, oth, LessNumber[int8], MoreNumber[int8]))
	s.Describe("int16", SpecCompare[int16](act, val, oth, LessNumber[int16], MoreNumber[int16]))
	s.Describe("int32", SpecCompare[int32](act, val, oth, LessNumber[int32], MoreNumber[int32]))
	s.Describe("int64", SpecCompare[int64](act, val, oth, LessNumber[int64], MoreNumber[int64]))

	s.Describe("uint", SpecCompare[uint](act, val, oth, LessNumber[uint], MoreNumber[uint]))
	s.Describe("uint8", SpecCompare[uint8](act, val, oth, LessNumber[uint8], MoreNumber[uint8]))
	s.Describe("uint16", SpecCompare[uint16](act, val, oth, LessNumber[uint16], MoreNumber[uint16]))
	s.Describe("uint32", SpecCompare[uint32](act, val, oth, LessNumber[uint32], MoreNumber[uint32]))
	s.Describe("uint64", SpecCompare[uint64](act, val, oth, LessNumber[uint64], MoreNumber[uint64]))

	s.Describe("float", func(s *testcase.Spec) {
		fl32 := let.Var(s, func(t *testcase.T) float32 {
			return t.Random.Float32()
		})

		fl64 := let.Var(s, func(t *testcase.T) float64 {
			return t.Random.Float64()
		})

		s.Context("float32", SpecCompare[float32](act, val, oth,
			func(t *testcase.T) float32 {
				return fl32.Get(t) - 1.0
			}, func(t *testcase.T) float32 {
				return fl32.Get(t) + 1.0
			}))

		s.Context("float64", SpecCompare[float64](act, val, oth,
			func(t *testcase.T) float64 {
				return fl64.Get(t) - 1.0
			}, func(t *testcase.T) float64 {
				return fl64.Get(t) + 1.0
			}))
	})

	s.Describe("time.Time", func(s *testcase.Spec) {
		tme := let.Var(s, func(t *testcase.T) time.Time {
			return t.Random.Time()
		})

		SpecCompare[time.Time](act, val, oth,
			func(t *testcase.T) time.Time {
				return tme.Get(t).Add(t.Random.DurationBetween(time.Second, time.Hour) * -1)
			}, func(t *testcase.T) time.Time {
				return tme.Get(t).Add(t.Random.DurationBetween(time.Second, time.Hour))
			})(s)
	})

	s.Describe("string", func(s *testcase.Spec) {
		length := let.IntB(s, 3, 7)

		SpecCompare[string](act, val, oth,
			func(t *testcase.T) string {
				return t.Random.StringNWithCharset(length.Get(t), "abcdefghijklm")
			},
			func(t *testcase.T) string {
				return t.Random.StringNWithCharset(length.Get(t), "nopqrstuvwxyz")
			})(s)
	})

	s.When("values are not the same type", func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) reflect.Value {
			return random.Pick(t.Random,
				reflect.ValueOf(t.Random.Int()),
				reflect.ValueOf(t.Random.Float32()),
				reflect.ValueOf(t.Random.Float64()),
			)
		})

		oth.Let(s, func(t *testcase.T) reflect.Value {
			return random.Pick(t.Random,
				reflect.ValueOf(t.Random.String()),
				reflect.ValueOf(t.Random.Time()),
			)
		})

		s.Then("it is expected to return a type mismatch error", func(t *testcase.T) {
			_, err := act(t)

			assert.ErrorIs(t, err, reflectkit.ErrTypeMismatch)
		})
	})

	s.Describe("Comparable implementation", func(s *testcase.Spec) {
		var _ reflectkit.Comparable[time.Time] = time.Time{}

		tme := let.Var(s, func(t *testcase.T) time.Time {
			return t.Random.Time()
		})

		SpecCompare[time.Time](act, val, oth,
			func(t *testcase.T) time.Time {
				return tme.Get(t).Add(t.Random.DurationBetween(time.Second, time.Hour) * -1)
			}, func(t *testcase.T) time.Time {
				return tme.Get(t).Add(t.Random.DurationBetween(time.Second, time.Hour))
			})(s)
	})

	s.Describe("Cmp implementation", func(s *testcase.Spec) {
		var _ reflectkit.CmpComparable[*big.Int] = (*big.Int)(nil)

		ref := let.Var(s, func(t *testcase.T) int64 {
			return int64(t.Random.Int())
		})

		SpecCompare[*big.Int](act, val, oth,
			func(t *testcase.T) *big.Int {
				bigInt := big.NewInt(ref.Get(t))
				return bigInt.Sub(bigInt, big.NewInt(100))
			}, func(t *testcase.T) *big.Int {
				bigInt := big.NewInt(ref.Get(t))
				return bigInt.Add(bigInt, big.NewInt(100))
			})(s)
	})

	// s.When("value is uint", SpecCompare[uint](act, val, oth, func(t *testcase.T) uint {
	// 	return uint(t.Random.IntBetween(0, 100))
	// }, func(t *testcase.T) uint {
	// 	return uint(t.Random.IntBetween(101, 200))
	// }))
}

type number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64
}

func LessNumber[T number](t *testcase.T) T {
	var min, max int8 // the smalest accepted number value type
	min = 0
	max = 4
	return T(t.Random.IntBetween(int(min), int(max)))
}

func MoreNumber[T number](t *testcase.T) T {
	var min, max int8 // the smalest accepted number value type
	min = 5
	max = 10
	return T(t.Random.IntBetween(int(min), int(max)))
}

func SpecCompare[T any](act func(t *testcase.T) (int, error), val, oth testcase.Var[reflect.Value], less, more func(t *testcase.T) T) func(s *testcase.Spec) {
	return func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.OnFail(func() {
				t.LogPretty("val:", val.Get(t).Interface())
				t.LogPretty("oth:", oth.Get(t).Interface())
			})
		})

		var be = func(fn func(*testcase.T) T) func(*testcase.T) reflect.Value {
			return func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(fn(t))
			}
		}

		s.And("val is less than oth", func(s *testcase.Spec) {
			val.Let(s, be(less))
			oth.Let(s, be(more))

			s.Then("vall is reported as less compared to oth", func(t *testcase.T) {
				c, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, c, -1)
			})
		})

		s.And("val is eq than oth", func(s *testcase.Spec) {
			val.Let(s, be(less))
			oth.Let(s, val.Get)

			s.Then("vall is reported as eq compared to oth", func(t *testcase.T) {
				c, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, c, 0)
			})
		})

		s.And("val is more than oth", func(s *testcase.Spec) {
			val.Let(s, be(more))
			oth.Let(s, be(less))

			s.Then("val is reported as more compared to oth", func(t *testcase.T) {
				c, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, c, 1)
			})
		})
	}
}

func TestIsNilable(t *testing.T) {
	t.Run("reflec.Kind", func(t *testing.T) {
		assert.False(t, reflectkit.IsNilable(reflect.Invalid), "Invalid")
		assert.False(t, reflectkit.IsNilable(reflect.Bool), "Bool")
		assert.False(t, reflectkit.IsNilable(reflect.Int), "Int")
		assert.False(t, reflectkit.IsNilable(reflect.Int8), "Int8")
		assert.False(t, reflectkit.IsNilable(reflect.Int16), "Int16")
		assert.False(t, reflectkit.IsNilable(reflect.Int32), "Int32")
		assert.False(t, reflectkit.IsNilable(reflect.Int64), "Int64")
		assert.False(t, reflectkit.IsNilable(reflect.Uint), "Uint")
		assert.False(t, reflectkit.IsNilable(reflect.Uint8), "Uint8")
		assert.False(t, reflectkit.IsNilable(reflect.Uint16), "Uint16")
		assert.False(t, reflectkit.IsNilable(reflect.Uint32), "Uint32")
		assert.False(t, reflectkit.IsNilable(reflect.Uint64), "Uint64")
		assert.False(t, reflectkit.IsNilable(reflect.Uintptr), "Uintptr")
		assert.False(t, reflectkit.IsNilable(reflect.Float32), "Float32")
		assert.False(t, reflectkit.IsNilable(reflect.Float64), "Float64")
		assert.False(t, reflectkit.IsNilable(reflect.Complex64), "Complex64")
		assert.False(t, reflectkit.IsNilable(reflect.Complex128), "Complex128")
		assert.False(t, reflectkit.IsNilable(reflect.Array), "Array")
		assert.True(t, reflectkit.IsNilable(reflect.Chan), "Chan")
		assert.True(t, reflectkit.IsNilable(reflect.Func), "Func")
		assert.True(t, reflectkit.IsNilable(reflect.Interface), "Interface")
		assert.True(t, reflectkit.IsNilable(reflect.Map), "Map")
		assert.True(t, reflectkit.IsNilable(reflect.Pointer), "Pointer")
		assert.True(t, reflectkit.IsNilable(reflect.Slice), "Slice")
		assert.False(t, reflectkit.IsNilable(reflect.String), "String")
		assert.False(t, reflectkit.IsNilable(reflect.Struct), "Struct")
		assert.False(t, reflectkit.IsNilable(reflect.UnsafePointer), "UnsafePointer")
	})
	t.Run("reflect.Value - smoke", func(t *testing.T) {
		assert.True(t, reflectkit.IsNilable(reflect.ValueOf(map[string]struct{}{})))
		assert.False(t, reflectkit.IsNilable(reflect.ValueOf("foo bar baz")))
	})
}

func TestToType(t *testing.T) {
	t.Run("Given a reflect.Type as input", func(t *testing.T) {
		expected := reflect.TypeOf(0)
		actual := reflectkit.ToType(expected)
		assert.Equal(t, expected, actual)
	})

	t.Run("When given a reflect.Value containing an integer", func(t *testing.T) {
		value := reflect.ValueOf(42)
		expected := value.Type()
		actual := reflectkit.ToType(value)
		assert.Equal(t, expected, actual)
	})

	t.Run("Given a string instance (default path)", func(t *testing.T) {
		input := "hello"
		expected := reflect.TypeOf(input)
		actual := reflectkit.ToType(input)
		assert.Equal(t, expected, actual)
	})

	t.Run("When input is a nil pointer (default path)", func(t *testing.T) {
		var ptr *int
		expected := reflect.TypeOf(ptr)
		actual := reflectkit.ToType(ptr)
		assert.Equal(t, expected, actual)
	})

	t.Run("Invalid reflect.Value panics", func(t *testing.T) {
		val := reflect.Value{} // invalid Value (zeroed)
		out := assert.Panic(t, func() { reflectkit.ToType(val) })
		err, ok := out.(error)
		assert.True(t, ok, "error panic value was expected")
		assert.Contain(t, err.Error(), "Value.Type")
	})
}
