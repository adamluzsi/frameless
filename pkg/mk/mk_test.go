package mk_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/mk"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase/assert"
)

func ExampleNew() {
	v := mk.New[TypeWithInit]()

	_ = v
}

func ExampleNew_nested() {
	v := mk.New[NestedType]()
	_ = v        // initialised
	_ = v.Nested // initialised
}

func TestNew(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		assert.Equal(t, mk.New[int](), new(int))
		assert.Equal(t, mk.New[string](), new(string))
	})
	t.Run("type with Init", func(t *testing.T) {
		v := mk.New[TypeWithInit]()
		assert.Equal(t, v.V1, "V1i")
		assert.Equal(t, v.V2, "V2i")
	})
	t.Run("type with default tags", func(t *testing.T) {
		v := mk.New[TypeWithDefaultTags]()
		assert.Equal(t, v.Foo, "foo")
		assert.Equal(t, v.Bar, 42)
		assert.Equal(t, v.Baz, true)
	})
	t.Run("struct type with nested init", func(t *testing.T) {
		v := mk.New[NestedType]()
		assert.Equal(t, v.Nested.V1, "V1i")
		assert.Equal(t, v.Nested.V2, "V2i")
		assert.Equal(t, v.V1, "NT:V1i")
	})
}

func TestReflectNew(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		assert.Equal(t, mk.ReflectNew(reflectkit.TypeOf[int]()).Interface().(*int), new(int))
		assert.Equal(t, mk.ReflectNew(reflectkit.TypeOf[string]()).Interface().(*string), new(string))
	})
	t.Run("type with Init", func(t *testing.T) {
		T := reflectkit.TypeOf[TypeWithInit]()
		v := mk.ReflectNew(T).Interface().(*TypeWithInit)
		assert.Equal(t, v.V1, "V1i")
		assert.Equal(t, v.V2, "V2i")
	})
	t.Run("struct type with nested init", func(t *testing.T) {
		v := mk.ReflectNew(reflectkit.TypeOf[NestedType]()).Interface().(*NestedType)
		assert.Equal(t, v.Nested.V1, "V1i")
		assert.Equal(t, v.Nested.V2, "V2i")
		assert.Equal(t, v.V1, "NT:V1i")
	})
}

func BenchmarkNew(b *testing.B) {
	b.Run("new", func(b *testing.B) {
		var v *TypeWithInit
		for i := 0; i < b.N; i++ {
			v = new(TypeWithInit)
			v.Init()
		}
	})
	b.Run("mk.New", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mk.New[TypeWithInit]()
		}
	})
	b.Run("nested", func(b *testing.B) {
		b.Run("new", func(b *testing.B) {
			var v *NestedType
			for i := 0; i < b.N; i++ {
				v = new(NestedType)
				v.Init()
				v.Nested.Init()
			}
		})
		b.Run("mk.New", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				mk.New[NestedType]()
			}
		})
	})
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

type NestedType struct {
	V1     string
	Nested TypeWithInit
}

func (nt *NestedType) Init() {
	nt.V1 = "NT:" + nt.Nested.V1
}

type TypeWithDefaultTags struct {
	Foo string `default:"foo"`
	Bar int    `default:"42"`
	Baz bool   `default:"true"`
}
