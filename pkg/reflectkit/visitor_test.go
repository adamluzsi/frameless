package reflectkit_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/refnode"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestVisitValues(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("struct", func(t *testcase.T) {
		type T struct {
			A int
			B int
		}

		var v = T{A: t.Random.Int()}

		rv := reflect.ValueOf(v)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		assert.Equal(t, len(vs), 3)

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.Equal(t, got.NodeType, refnode.Struct)
			assert.Equal[any](t, v, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.StructField))
			assert.Equal(t, "A", got.StructField.Name)
			assert.Equal[any](t, v.A, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.StructField))
			assert.Equal(t, "B", got.StructField.Name)
			assert.Equal[any](t, v.B, got.Value.Interface())
		})
	})

	s.Test("array", func(t *testcase.T) {
		type T [4]int

		var in = T{1, 2, 3}

		rv := reflect.ValueOf(in)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		assert.Equal(t, len(vs), 5, "array[4] + the 4 element")

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.Array))
			assert.True(t, got.Path().Contains(refnode.Array))
			assert.False(t, got.Path().Contains(refnode.Array, refnode.ArrayElem))
			assert.Equal[any](t, in, got.Value.Interface())
		})

		for i, n := range in {
			assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.ArrayElem))
				assert.True(t, got.Path().Contains(refnode.Array))
				assert.True(t, got.Path().Contains(refnode.Array, refnode.ArrayElem))
				assert.Equal(t, got.Index, i)
				assert.Equal[any](t, n, got.Value.Interface())
			})
		}
	})

	s.Test("slice", func(t *testcase.T) {
		type T []int

		var (
			length   = t.Random.IntBetween(3, 7)
			input  T = random.Slice(length, t.Random.Int)
		)

		rv := reflect.ValueOf(input)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		assert.Equal(t, len(vs), 1+length, "one for slice plus the length of slice (elements)")

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.Slice))
			assert.True(t, got.Path().Contains(refnode.Slice))
			assert.False(t, got.Path().Contains(refnode.Slice, refnode.SliceElem))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		for i, n := range input {
			assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.SliceElem))
				assert.True(t, got.Path().Contains(refnode.Slice))
				assert.True(t, got.Path().Contains(refnode.Slice, refnode.SliceElem))
				assert.Equal(t, got.Index, i)
				assert.Equal[any](t, n, got.Value.Interface())
			})
		}
	})

	s.Test("map", func(t *testcase.T) {
		type T map[string]int

		var (
			length   = t.Random.IntBetween(3, 7)
			input  T = random.Map(length, func() (string, int) {
				return t.Random.HexN(8), t.Random.Int()
			})
		)

		rv := reflect.ValueOf(input)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		assert.Equal(t, len(vs), 1+length+length, "map + its keys and values")

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.Map))
			assert.True(t, got.Path().Contains(refnode.Map))
			assert.False(t, got.Path().Contains(refnode.Map, refnode.MapKey))
			assert.False(t, got.Path().Contains(refnode.Map, refnode.MapValue))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		for mKey, mVal := range input {
			assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.MapKey))
				assert.True(t, got.Path().Contains(refnode.Map, refnode.MapKey))
				assert.False(t, got.Path().Contains(refnode.Map, refnode.MapValue))
				assert.Equal[any](t, mKey, got.Value.Interface())
			})

			assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.MapValue))
				assert.False(t, got.Path().Contains(refnode.Map, refnode.MapKey))
				assert.True(t, got.Path().Contains(refnode.Map, refnode.MapValue))
				assert.Equal[any](t, mVal, got.Value.Interface())
			})
		}
	})

	s.Test("pointer", func(t *testcase.T) {
		var input = pointer.Of(t.Random.Int())

		rv := reflect.ValueOf(input)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		assert.Equal(t, len(vs), 2, "pointer + value")

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.Equal(t, got.NodeType, refnode.Pointer)
			assert.True(t, got.Path().Contains(refnode.Pointer))
			assert.False(t, got.Path().Contains(refnode.Pointer, refnode.PointerElem))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.PointerElem))
			assert.True(t, got.Path().Contains(refnode.Pointer, refnode.PointerElem))
			assert.False(t, got.Path().Contains(refnode.PointerElem, refnode.Pointer))
			assert.Equal[any](t, *input, got.Value.Interface())
		})
	})

	s.Test("interface", func(t *testcase.T) {
		var (
			RType = reflectkit.TypeOf[testent.Fooer]()
			FooT  = reflectkit.TypeOf[testent.Foo]()
			input = testent.Foo{
				ID:  testent.FooID(t.Random.HexN(4)),
				Foo: "foo",
				Bar: "bar",
				Baz: "baz",
			}
		)

		var x testent.Fooer = input
		rv := reflect.ValueOf(&x).Elem()
		assert.Equal(t, rv.Type(), RType)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		interValVisitCount := iterkit.Count(reflectkit.VisitValues(reflect.ValueOf(input)))
		assert.Equal(t, len(vs), 1+interValVisitCount, "interface + interface value")

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.Equal(t, got.NodeType, refnode.Interface)
			assert.Equal(t, got.Value.Kind(), reflect.Interface)
			assert.Equal[any](t, x, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.Equal(t, FooT, got.Value.Type())
			assert.True(t, got.Is(refnode.InterfaceElem))
			assert.Equal(t, got.Value.Kind(), FooT.Kind())
			assert.Equal(t, got.Value.Type(), FooT)
			assert.Equal[any](t, input, got.Value.Interface())
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		type Baz struct {
			V string
		}
		type Bar struct {
			Bazs []Baz
		}
		type Foo struct {
			Bar *Bar
		}
		var v Foo = Foo{
			Bar: &Bar{
				Bazs: []Baz{
					Baz{V: "foo"},
					Baz{V: "bar"},
					Baz{V: "baz"},
				},
			},
		}

		rv := reflect.ValueOf(v)
		vs := iterkit.Collect(reflectkit.VisitValues(rv))

		var bazVs []string
		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.StructField))
			assert.Equal(t, got.StructField.Name, "V")
			bazVs = append(bazVs, got.Value.String())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reflectkit.V) {
			assert.True(t, got.Is(refnode.PointerElem))
		})
	})

	s.Context("V#Is", func(s *testcase.Spec) {
		s.Test("slice elem", func(t *testcase.T) {
			var vs []int = []int{1, 2, 3}
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(vs)))

			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.SliceElem))
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("array elem", func(t *testcase.T) {
			var vs [3]int = [3]int{1, 2, 3}
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(vs)))

			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.ArrayElem))
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("struct field", func(t *testcase.T) {
			type T struct {
				V string
			}
			elemType := reflectkit.TypeOf[string]()

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(T{V: t.Random.HexN(4)})))

			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.True(t, got.Is(refnode.StructField))
				assert.NotEmpty(t, got.StructField)
				assert.Equal(t, got.StructField.Name, "V")
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("pointer elem", func(t *testcase.T) {
			var n int = t.Random.Int()
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(&n)))

			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.Equal(t, got.Value.Type(), elemType)
				assert.True(t, got.Is(refnode.PointerElem))
				assert.False(t, got.Is(refnode.Pointer))
			})
		})

		s.Test("interface elem", func(t *testcase.T) {
			type I any

			var n int = t.Random.Int()
			var i I = n

			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(&i).Elem()))

			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.Equal(t, got.Value.Type(), elemType)
				assert.True(t, got.Is(refnode.InterfaceElem))
				assert.False(t, got.Is(refnode.Interface))
				assert.True(t, got.Path().Contains(refnode.Interface, refnode.InterfaceElem))
			})
		})

		s.Test("nested pointer elem", func(t *testcase.T) {
			type X interface{}

			var (
				n int = t.Random.Int()
				x X   = n
				v *X  = &x
			)

			vvs := iterkit.Collect(reflectkit.VisitValues(reflect.ValueOf(v)))

			assert.Equal(t, 3, len(vvs), "pointer -> interface -> int")

			var expectedValueType = reflectkit.TypeOf(n)
			assert.OneOf(t, vvs, func(t testing.TB, got reflectkit.V) {
				assert.Equal(t, got.Value.Type(), expectedValueType)
				assert.NotEqual(t, got.Value.Kind(), reflect.Interface)

				assert.True(t, got.Path().Contains(
					refnode.Pointer, refnode.PointerElem,
					refnode.Interface, refnode.InterfaceElem,
				))
				assert.False(t, got.Path().Contains(
					refnode.Interface, refnode.InterfaceElem,
					refnode.Pointer, refnode.PointerElem,
				))

				assert.NotNil(t, got.Parent)

				assert.True(t, got.Is(refnode.PointerElem))
				assert.True(t, got.Is(refnode.InterfaceElem))

				assert.False(t, got.Is(refnode.Pointer))
				assert.False(t, got.Is(refnode.Interface))

				assert.Equal(t, got.Value.Type(), expectedValueType)
				assert.Equal[any](t, got.Value.Interface(), n)
			})
		})
	})
}
