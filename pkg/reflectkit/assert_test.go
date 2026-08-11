package reflectkit_test

import (
	"fmt"
	"iter"
	"reflect"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type StructExampleAssertMethodSimple struct{}

func (StructExampleAssertMethodSimple) IntToString(n int) string {
	return strconv.Itoa(n)
}
func ExampleAssertMethod() {
	receiver := reflect.ValueOf(StructExampleAssertMethodSimple{})

	intToString, ok := reflectkit.AssertMethod[func(int) string](receiver, "IntToString")
	if !ok {
		panic("expected IntToString to match func(int) string")
	}

	fmt.Println(intToString(42))
	// Output: 42
}

type StructExampleAssertMethodGeneric[F testent.Fooer] struct{}

func (StructExampleAssertMethodGeneric[F]) MethodName() F {
	var zero F
	return zero
}

func ExampleAssertMethod_interfaceReturn() {
	// receiver type is initialized with Foo "somewhere else"
	receiver := reflect.ValueOf(StructExampleAssertMethodGeneric[testent.Foo]{})

	getFooer, ok := reflectkit.AssertMethod[func() testent.Fooer](receiver, "MethodName")
	if !ok {
		panic("expected MethodName to match func() testent.Fooer signature")
	}

	fmt.Println(getFooer().GetFoo())
}

func TestAssertMethod(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("non-matching assertion", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) string](receiver, "NonExistent")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("1:1 function signature", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) string](receiver, "IntToString")
		assert.True(t, ok)
		assert.NotNil(t, fn)

		n := t.Random.Int()
		assert.Equal(t, strconv.Itoa(n), fn(n))
	})

	s.Test("1:1 function signature with typed func type", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		type FN func(int) string
		fn, ok := reflectkit.AssertMethod[FN](receiver, "IntToString")
		assert.True(t, ok)
		assert.NotNil(t, fn)

		n := t.Random.Int()
		assert.Equal(t, strconv.Itoa(n), fn(n))
	})

	s.Test("function signature's return interface value is matched due to interface is implemented by the actual function's return type", func(t *testcase.T) {
		var foo = t.Random.String()
		receiver := reflect.ValueOf(AssertMethodSubject{
			Foo: testent.Foo{Foo: foo},
		})

		fn, ok := reflectkit.AssertMethod[func() testent.Fooer](receiver, "F")
		assert.True(t, ok)
		assert.NotNil(t, fn)

		var fooer testent.Fooer
		assert.NotPanic(t, func() { fooer = fn() })
		assert.NotNil(t, fooer)
		assert.Equal(t, fooer.GetFoo(), foo)
	})

	s.Test("the type argument is not a function type", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		_, ok := reflectkit.AssertMethod[int](receiver, "IntToString")
		assert.False(t, ok)
	})

	s.Test("the receiver is an invalid reflect.Value", func(t *testcase.T) {
		fn, ok := reflectkit.AssertMethod[func(int) string](reflect.Value{}, "IntToString")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("the number of input parameters differs", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int, int) string](receiver, "IntToString")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("the number of return values differs", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) (string, error)](receiver, "IntToString")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("an input parameter type is incompatible", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(string) string](receiver, "IntToString")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a return value type is incompatible", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) int](receiver, "IntToString")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a method with multiple return values", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) (string, error)](receiver, "Multi")
		assert.True(t, ok)
		assert.NotNil(t, fn)

		n := random.New(random.CryptoSeed{}).Int()
		got, err := fn(n)
		assert.NoError(t, err)
		assert.Equal(t, strconv.Itoa(n), got)
	})

	s.Test("a method without return values", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int)](receiver, "NoReturn")
		assert.True(t, ok)
		assert.NotNil(t, fn)
		assert.NotPanic(t, func() { fn(42) })
	})

	s.Test("a variadic method matched with the same variadic signature", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(...int) int](receiver, "Sum")
		assert.True(t, ok)
		assert.NotNil(t, fn)
		assert.Equal(t, 6, fn(1, 2, 3))
	})

	s.Test("a variadic method does not match a non-variadic signature", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func([]int) int](receiver, "Sum")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a concrete argument is accepted where the method expects an interface (contravariance)", func(t *testcase.T) {
		foo := random.New(random.CryptoSeed{}).String()
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(testent.Foo) string](receiver, "FromFooer")
		assert.True(t, ok)
		assert.NotNil(t, fn)
		assert.Equal(t, foo, fn(testent.Foo{Foo: foo}))
	})

	s.Test("an interface argument is rejected where the method expects a concrete type", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(testent.Fooer) string](receiver, "FromFoo")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a concrete return type is rejected where the requested type is an unrelated interface", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() error](receiver, "F")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a method mixing a normal argument with a variadic argument", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(string, ...int) string](receiver, "Join")
		assert.True(t, ok)
		assert.NotNil(t, fn)
		assert.Equal(t, "n=6", fn("n=", 1, 2, 3))
	})

	s.Test("a pointer receiver method is not part of a value receiver's method set", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() string](receiver, "PtrOnly")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})

	s.Test("a pointer receiver method is found on a pointer receiver", func(t *testcase.T) {
		receiver := reflect.ValueOf(&AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() string](receiver, "PtrOnly")
		assert.True(t, ok)
		assert.NotNil(t, fn)
		assert.Equal(t, "ptr", fn())
	})

	s.Test("iterator with concrete type that implements a contract can be used with the contract interface as type", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() iter.Seq2[ds.KeyValuePair[string, int], error]](receiver, "KVE")
		assert.True(t, ok)

		kvs, err := iterkit.CollectE(fn())
		assert.NoError(t, err)

		assert.OneOf(t, kvs, func(t testing.TB, kv ds.KeyValuePair[string, int]) {
			assert.NotNil(t, kv)
			assert.Equal(t, kv.Key(), "42")
			assert.Equal(t, kv.Value(), 42)
		})
	})

	s.Test("return value is a struct type with type parameter a contract, then through the contract of the type parameter, it can be referenced", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() AssertMethodSubjectGenSub[testent.Fooer]](receiver, "GetFooerStruct")
		assert.True(t, ok)

		var v1 = fn()
		assert.Equal[testent.Fooer](t, v1.Fooer, testent.Foo{
			ID:  "42",
			Foo: "foo",
			Bar: "bar",
			Baz: "baz",
		})
	})

	s.Test("return value is a struct type, all fields match with another struct type, then they are interchangeable in the signature usage", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() AssertMethodSubjectGenSubAlt](receiver, "GetFooerStruct")
		assert.True(t, ok)

		var v1 = fn()
		assert.Equal[testent.Fooer](t, v1.Fooer, testent.Foo{
			ID:  "42",
			Foo: "foo",
			Bar: "bar",
			Baz: "baz",
		})
	})

	s.Test("return value is a struct type with type parameter a contract, but unrelated to the given function signature", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func() AssertMethodSubjectGenSubUnrelated[testent.Fooer]](receiver, "GetFooerStruct")
		assert.False(t, ok)
		assert.Nil(t, fn)
	})
}

type AssertMethodSubject struct {
	Foo testent.Foo
}

type AssertMethodSubjectGenSub[Fooer testent.Fooer] struct {
	Fooer Fooer
}

func (s AssertMethodSubject) GetFooerStruct() AssertMethodSubjectGenSub[testent.Foo] {
	return AssertMethodSubjectGenSub[testent.Foo]{
		Fooer: testent.Foo{
			ID:  "42",
			Foo: "foo",
			Bar: "bar",
			Baz: "baz",
		},
	}
}

type AssertMethodSubjectGenSubAlt struct {
	Fooer testent.Fooer
}

type AssertMethodSubjectGenSubUnrelated[Fooer testent.Fooer] struct{}

func (s AssertMethodSubject) IntToString(n int) string { return strconv.Itoa(n) }

func (s AssertMethodSubject) F() testent.Foo {
	return s.Foo
}

func (s AssertMethodSubject) Multi(n int) (string, error) {
	return strconv.Itoa(n), nil
}

func (s AssertMethodSubject) NoReturn(n int) {}

func (s AssertMethodSubject) Sum(ns ...int) int {
	var total int
	for _, n := range ns {
		total += n
	}
	return total
}

func (s AssertMethodSubject) Join(prefix string, ns ...int) string {
	return prefix + strconv.Itoa(s.Sum(ns...))
}

func (s AssertMethodSubject) FromFooer(f testent.Fooer) string { return f.GetFoo() }

func (s AssertMethodSubject) FromFoo(f testent.Foo) string { return f.GetFoo() }

func (s *AssertMethodSubject) PtrOnly() string { return "ptr" }

func (s AssertMethodSubject) KVE() iter.Seq2[ds.KV[string, int], error] {
	return func(yield func(ds.KV[string, int], error) bool) {
		yield(ds.KV[string, int]{K: "42", V: 42}, nil)
	}
}
