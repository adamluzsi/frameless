package reflectkit_test

import (
	"iter"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestVisit(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		v = let.Var[reflect.Value](s, nil)
	)
	act := let.Act(func(t *testcase.T) iter.Seq[reftree.Node] {
		return reflectkit.Visit(v.Get(t))
	})

	var collect = func(t *testcase.T) []reftree.Node {
		return iterkit.Collect(act(t))
	}

	s.Test("struct", func(t *testcase.T) {
		type T struct {
			A int
			B int
		}

		var v = T{A: t.Random.Int()}

		rv := reflect.ValueOf(v)
		vs := iterkit.Collect(reflectkit.Visit(rv))

		assert.Equal(t, len(vs), 3)

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.Equal(t, got.Type, reftree.Struct)
			assert.Equal[any](t, v, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.StructField))
			assert.Equal(t, "A", got.StructField.Name)
			assert.Equal[any](t, v.A, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.StructField))
			assert.Equal(t, "B", got.StructField.Name)
			assert.Equal[any](t, v.B, got.Value.Interface())
		})
	})

	s.Test("array", func(t *testcase.T) {
		type T [4]int

		var in = T{1, 2, 3}

		rv := reflect.ValueOf(in)
		vs := iterkit.Collect(reflectkit.Visit(rv))

		assert.Equal(t, len(vs), 5, "array[4] + the 4 element")

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.Array))
			assert.True(t, got.Path().Contains(reftree.Array))
			assert.False(t, got.Path().Contains(reftree.Array, reftree.ArrayElem))
			assert.Equal[any](t, in, got.Value.Interface())
		})

		for i, n := range in {
			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.ArrayElem))
				assert.True(t, got.Path().Contains(reftree.Array))
				assert.True(t, got.Path().Contains(reftree.Array, reftree.ArrayElem))
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
		vs := iterkit.Collect(reflectkit.Visit(rv))

		assert.Equal(t, len(vs), 1+length, "one for slice plus the length of slice (elements)")

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.Slice))
			assert.True(t, got.Path().Contains(reftree.Slice))
			assert.False(t, got.Path().Contains(reftree.Slice, reftree.SliceElem))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		for i, n := range input {
			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.SliceElem))
				assert.True(t, got.Path().Contains(reftree.Slice))
				assert.True(t, got.Path().Contains(reftree.Slice, reftree.SliceElem))
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
		vs := iterkit.Collect(reflectkit.Visit(rv))

		assert.Equal(t, len(vs), 1+length+length, "map + its keys and values")

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.Map))
			assert.True(t, got.Path().Contains(reftree.Map))
			assert.False(t, got.Path().Contains(reftree.Map, reftree.MapKey))
			assert.False(t, got.Path().Contains(reftree.Map, reftree.MapValue))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		for mKey, mVal := range input {
			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.MapKey))
				assert.True(t, got.Path().Contains(reftree.Map, reftree.MapKey))
				assert.False(t, got.Path().Contains(reftree.Map, reftree.MapValue))
				assert.Equal[any](t, mKey, got.Value.Interface())
			})

			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.MapValue))
				assert.False(t, got.Path().Contains(reftree.Map, reftree.MapKey))
				assert.True(t, got.Path().Contains(reftree.Map, reftree.MapValue))
				assert.Equal[any](t, mVal, got.Value.Interface())
			})
		}
	})

	s.Test("pointer", func(t *testcase.T) {
		var input = pointer.Of(t.Random.Int())

		rv := reflect.ValueOf(input)
		vs := iterkit.Collect(reflectkit.Visit(rv))

		assert.Equal(t, len(vs), 2, "pointer + value")

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.Equal(t, got.Type, reftree.Pointer)
			assert.True(t, got.Path().Contains(reftree.Pointer))
			assert.False(t, got.Path().Contains(reftree.Pointer, reftree.PointerElem))
			assert.Equal[any](t, input, got.Value.Interface())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.PointerElem))
			assert.True(t, got.Path().Contains(reftree.Pointer, reftree.PointerElem))
			assert.False(t, got.Path().Contains(reftree.PointerElem, reftree.Pointer))
			assert.Equal[any](t, *input, got.Value.Interface())
		})
	})

	s.Context("interface", func(s *testcase.Spec) {
		concrete := let.Var(s, func(t *testcase.T) testent.Foo {
			return testent.Foo{
				ID:  testent.FooID(t.Random.HexN(4)),
				Foo: t.Random.String(),
				Bar: t.Random.String(),
				Baz: t.Random.String(),
			}
		})
		v.Let(s, func(t *testcase.T) reflect.Value {
			var x testent.Fooer = concrete.Get(t)
			rv := reflect.ValueOf(&x).Elem()
			assert.Equal(t, rv.Type(), reflectkit.TypeOf[testent.Fooer]())
			return rv
		})

		s.Then("it will visit the interface, and its value both", func(t *testcase.T) {
			wInterfaceVisitCount := iterkit.Count(act(t))
			wValueVisitCount := iterkit.Count(reflectkit.Visit(reflect.ValueOf(concrete.Get(t))))
			assert.Equal(t, wInterfaceVisitCount, 1+wValueVisitCount, "interface + interface values")
		})

		s.Then("the visited values will contain the interface node", func(t *testcase.T) {
			vs := collect(t)

			FooerT := reflectkit.TypeOf[testent.Fooer]()
			assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
				assert.Equal(tb, got.Type, reftree.Interface)
				assert.Equal(tb, got.Value.Kind(), reflect.Interface)
				assert.Equal(tb, got.Value.Type(), FooerT)
				assert.Equal[any](tb, concrete.Get(t), got.Value.Interface())
			})
		})

		s.Then("the visited values will contain the interface elem node", func(t *testcase.T) {
			vs := collect(t)

			FooT := reflectkit.TypeOf[testent.Foo]()
			assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
				assert.Equal(tb, FooT, got.Value.Type())
				assert.True(tb, got.Is(reftree.InterfaceElem))
				assert.Equal(tb, got.Value.Kind(), FooT.Kind())
				assert.Equal(tb, got.Value.Type(), FooT)
				assert.Equal[any](tb, concrete.Get(t), got.Value.Interface())
			})
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
		vs := iterkit.Collect(reflectkit.Visit(rv))

		var bazVs []string
		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.StructField))
			assert.Equal(t, got.StructField.Name, "V")
			bazVs = append(bazVs, got.Value.String())
		})

		assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
			assert.True(t, got.Is(reftree.PointerElem))
		})
	})

	s.Context("V#Is", func(s *testcase.Spec) {
		s.Test("slice elem", func(t *testcase.T) {
			var vs []int = []int{1, 2, 3}
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(vs)))

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.SliceElem))
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("array elem", func(t *testcase.T) {
			var vs [3]int = [3]int{1, 2, 3}
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(vs)))

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.ArrayElem))
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("struct field", func(t *testcase.T) {
			type T struct {
				V string
			}
			elemType := reflectkit.TypeOf[string]()

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(T{V: t.Random.HexN(4)})))

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.StructField))
				assert.NotEmpty(t, got.StructField)
				assert.Equal(t, got.StructField.Name, "V")
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("pointer elem", func(t *testcase.T) {
			var n int = t.Random.Int()
			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(&n)))

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.Equal(t, got.Value.Type(), elemType)
				assert.True(t, got.Is(reftree.PointerElem))
				assert.False(t, got.Is(reftree.Pointer))
			})
		})

		s.Test("interface elem", func(t *testcase.T) {
			type I any

			var n int = t.Random.Int()
			var i I = n

			elemType := reflectkit.TypeOf[int]()

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(&i).Elem()))

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.Equal(t, got.Value.Type(), elemType)
				assert.True(t, got.Is(reftree.InterfaceElem))
				assert.False(t, got.Is(reftree.Interface))
				assert.True(t, got.Path().Contains(reftree.Interface, reftree.InterfaceElem))
			})
		})

		s.Test("nested pointer elem", func(t *testcase.T) {
			type X interface{}

			var (
				n int = t.Random.Int()
				x X   = n
				v *X  = &x
			)

			vvs := iterkit.Collect(reflectkit.Visit(reflect.ValueOf(v)))

			assert.Equal(t, 3, len(vvs), "pointer -> interface -> int")

			var expectedValueType = reflectkit.TypeOf(n)
			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.Equal(t, got.Value.Type(), expectedValueType)
				assert.NotEqual(t, got.Value.Kind(), reflect.Interface)

				assert.True(t, got.Path().Contains(
					reftree.Pointer, reftree.PointerElem,
					reftree.Interface, reftree.InterfaceElem,
				))
				assert.False(t, got.Path().Contains(
					reftree.Interface, reftree.InterfaceElem,
					reftree.Pointer, reftree.PointerElem,
				))

				assert.NotNil(t, got.Parent)

				assert.True(t, got.Is(reftree.PointerElem))
				assert.True(t, got.Is(reftree.InterfaceElem))

				assert.False(t, got.Is(reftree.Pointer))
				assert.False(t, got.Is(reftree.Interface))

				assert.Equal(t, got.Value.Type(), expectedValueType)
				assert.Equal[any](t, got.Value.Interface(), n)
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Test("visited struct field can be set", func(t *testcase.T) {
			type T struct{ X string }

			var v T
			rv := reflect.ValueOf(&v).Elem()

			var vX reftree.Node
			for v := range reflectkit.Visit(rv) {
				if v.Type != reftree.StructField {
					continue
				}
				if v.StructField.Name == "X" {
					vX = v
					break
				}
			}
			vX.Value.Set(reflect.ValueOf("foo"))
			assert.Equal(t, v.X, "foo")
		})

		s.Test("setting a field value using the visited reflection value", func(t *testcase.T) {
			var (
				foo              = testent.MakeFoo(t)
				fooFooFieldValue reftree.Node
				found            bool
			)
			for v := range reflectkit.Visit(reflect.ValueOf(&foo)) {
				if v.Type != reftree.StructField {
					continue
				}
				if v.StructField.Name == "Foo" {
					fooFooFieldValue = v
					found = true
					break
				}
			}
			assert.True(t, found, assert.MessageF("expected that a %T has a Foo field", foo))

			exp := t.Random.Domain()
			fooFooFieldValue.Value.Set(reflect.ValueOf(exp))
			assert.Equal(t, foo.Foo, exp)
		})
	})
}
