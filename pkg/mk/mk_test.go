package mk_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/mk"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase/assert"
)

func ExampleNew() {
	v := mk.New[TypeWithInit]()

	_ = v
}

func TestNew(t *testing.T) {
	t.Run("Type with Init and Default", func(t *testing.T) {
		v := mk.New[TypeWithInitAndDefault]()
		assert.Equal(t, v.V1, "V1")
		assert.Equal(t, v.V2, "V2")
		assert.Equal(t, v.Foo, "foo")
		assert.Equal(t, v.Bar, 42)
		assert.Equal(t, v.Baz, true)
	})
	t.Run("primitive", func(t *testing.T) {
		assert.Equal(t, mk.New[int](), new(int))
		assert.Equal(t, mk.New[string](), new(string))
	})
	t.Run("type with Init only", func(t *testing.T) {
		v := mk.New[TypeWithInit]()
		assert.Equal(t, v.V1, "V1i")
		assert.Equal(t, v.V2, "V2i")
	})
	t.Run("type with Default only", func(t *testing.T) {
		v := mk.New[TypeWithDefault]()
		assert.Empty(t, v.V1)
		assert.Empty(t, v.V2)
		assert.Equal(t, v.Foo, "foo")
		assert.Equal(t, v.Bar, 42)
		assert.Equal(t, v.Baz, true)
	})
}

func TestReflectNew(t *testing.T) {
	t.Run("Type with Init and Default", func(t *testing.T) {
		v := mk.ReflectNew(reflect.TypeOf((*TypeWithInitAndDefault)(nil)).Elem()).Interface().(*TypeWithInitAndDefault)
		assert.Equal(t, v.V1, "V1")
		assert.Equal(t, v.V2, "V2")
		assert.Equal(t, v.Foo, "foo")
		assert.Equal(t, v.Bar, 42)
		assert.Equal(t, v.Baz, true)
	})
	t.Run("primitive", func(t *testing.T) {
		assert.Equal(t, mk.ReflectNew(reflectkit.TypeOf[int]()).Interface().(*int), new(int))
		assert.Equal(t, mk.ReflectNew(reflectkit.TypeOf[string]()).Interface().(*string), new(string))
	})
	t.Run("type with Init only", func(t *testing.T) {
		v := mk.ReflectNew(reflect.TypeOf((*TypeWithInit)(nil)).Elem()).Interface().(*TypeWithInit)
		assert.Equal(t, v.V1, "V1i")
		assert.Equal(t, v.V2, "V2i")
	})
	t.Run("type with Default only", func(t *testing.T) {
		v := mk.ReflectNew(reflect.TypeOf((*TypeWithDefault)(nil)).Elem()).Interface().(*TypeWithDefault)
		assert.Empty(t, v.V1)
		assert.Empty(t, v.V2)
		assert.Equal(t, v.Foo, "foo")
		assert.Equal(t, v.Bar, 42)
		assert.Equal(t, v.Baz, true)
	})
}

func BenchmarkNew(b *testing.B) {
	b.Run("new", func(b *testing.B) {
		var v *TypeWithInitAndDefault
		for i := 0; i < b.N; i++ {
			v = new(TypeWithInitAndDefault)
		}
		_ = v
	})
	b.Run("init+default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mk.New[TypeWithInitAndDefault]()
		}
	})
	b.Run("init", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mk.New[TypeWithInit]()
		}
	})
	b.Run("default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mk.New[TypeWithDefault]()
		}
	})
	b.Run("nested", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mk.New[NestedType]()
		}
	})
}

type TypeWithInitAndDefault struct {
	V1  string
	V2  string
	Foo string
	Bar int
	Baz bool
}

func (TypeWithInitAndDefault) Default() TypeWithInitAndDefault {
	return TypeWithInitAndDefault{
		Foo: "foo",
		Bar: 42,
		Baz: true,
		V2:  "will be overwritten in init",
	}
}

func (v *TypeWithInitAndDefault) Init() {
	v.V1 = "V1"
	if v.V2 != "will be overwritten in init" {
		panic("expected to have the default value")
	}
	v.V2 = "V2"
}

type TypeWithInit struct {
	V1  string
	V2  string
	Foo string
	Bar int
	Baz bool
}

func (v *TypeWithInit) Init() {
	v.V1 = "V1i"
	v.V2 = "V2i"
}

type TypeWithDefault struct {
	V1  string
	V2  string
	Foo string
	Bar int
	Baz bool
}

func (TypeWithDefault) Default() TypeWithDefault {
	return TypeWithDefault{
		Foo: "foo",
		Bar: 42,
		Baz: true,
	}
}

type NestedType struct {
	TypeWithInit
	TypeWithDefault
	TypeWithInitAndDefault
}
