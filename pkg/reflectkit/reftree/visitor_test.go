package reftree_test

import (
	"context"
	"iter"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/frameless/pkg/reflectkit/reftree/internal/reftestent"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestWalk(t *testing.T) {
	s := testcase.NewSpec(t)

	type Visitable struct{ V int }

	var makeVisitable = func(t *testcase.T) Visitable {
		return t.Random.Make(Visitable{}).(Visitable)
	}

	var (
		value = let.Var[reflect.Value](s, nil)
		visit = let.Var[reftree.VisitorFunc](s, nil)
	)
	act := let.Act(func(t *testcase.T) error {
		return reftree.Walk(value.Get(t), visit.Get(t))
	})

	var collectFrom = func(t *testcase.T, value reflect.Value) ([]reftree.Node, error) {
		var nodes []reftree.Node
		err := reftree.Walk(value, func(node reftree.Node) error {
			nodes = append(nodes, node)
			return nil
		})
		return nodes, err
	}

	var collect = func(t *testcase.T) ([]reftree.Node, error) {
		return collectFrom(t, value.Get(t))
	}

	s.Context("struct", func(s *testcase.Spec) {
		type T struct {
			A int
			B int
		}
		v := let.Var(s, func(t *testcase.T) T {
			return T{A: t.Random.Int()}
		})
		value.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(v.Get(t))
		})

		s.Test("struct", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			assert.Equal(t, len(vs), 3)

			expStruct := v.Get(t)

			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.Equal(t, got.Type, reftree.Struct)
				assert.Equal[any](t, expStruct, got.Value.Interface())
			})

			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.StructField))
				assert.Equal(t, "A", got.StructField.Name)
				assert.Equal[any](t, expStruct.A, got.Value.Interface())
			})

			assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.StructField))
				assert.Equal(t, "B", got.StructField.Name)
				assert.Equal[any](t, expStruct.B, got.Value.Interface())
			})
		})

		s.Test("Visitor#Skip can stop further down traversing during struct field iteration", func(t *testcase.T) {
			type A struct {
				V int
			}
			type B struct {
				A A
				V string
			}
			type C struct {
				B1 B
				B2 B
				V  float32
			}

			var (
				a1 = A{V: t.Random.Int()}
				a2 = A{V: t.Random.Int()}
				b1 = B{A: a1, V: t.Random.String()}
				b2 = B{A: a2, V: t.Random.String()}
				c0 = C{B1: b1, B2: b2, V: t.Random.Float32()}
			)

			var nodes []reftree.Node

			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.StructField && node.StructField.Name == "B1" {
					return reftree.Skip
				}
				return nil
			})

			value.Set(t, reflect.ValueOf(c0))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a1, node.Value.Interface())
			}, "a1 is not visited")

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a2, node.Value.Interface())
			}, "but a2 is visited since it was not step over")
		})

		s.Test("Visitor#Skip can stop further down traversing within a sub node of a iterated struct field", func(t *testcase.T) {
			type A struct {
				V int
			}
			type B struct {
				A A
				V string
			}
			type C struct {
				B1 B
				B2 B
				V  float32
			}

			var (
				a1 = A{V: t.Random.Int()}
				a2 = A{V: t.Random.Int()}
				b1 = B{A: a1, V: t.Random.String()}
				b2 = B{A: a2, V: t.Random.String()}
				c0 = C{B1: b1, B2: b2, V: t.Random.Float32()}
			)

			var nodes []reftree.Node

			visit.Set(t, func(node reftree.Node) error {
				if node.Type == reftree.Struct && node.Parent != nil && node.Parent.Type == reftree.StructField && node.Parent.StructField.Name == "A" {
					var ok bool
					for n := range node.IterUpward() {
						if n.Type == reftree.StructField && n.StructField.Name == "B1" {
							ok = true
						}
					}
					if ok {
						return reftree.Skip
					}
				}
				nodes = append(nodes, node)
				return nil
			})

			value.Set(t, reflect.ValueOf(c0))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a1, node.Value.Interface())
			}, "a1 is not visited")

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a2, node.Value.Interface())
			}, "but a2 is visited since it was not step over")
		})

		s.Test("Visitor#Skip interrupts the visiting of the struct field", func(t *testcase.T) {
			type A struct {
				V int
			}
			type B struct {
				A A
				V string
			}
			type C struct {
				B1 B
				B2 B
				V  float32
			}

			var (
				a1 = A{V: t.Random.Int()}
				a2 = A{V: t.Random.Int()}
				b1 = B{A: a1, V: t.Random.String()}
				b2 = B{A: a2, V: t.Random.String()}
				c0 = C{B1: b1, B2: b2, V: t.Random.Float32()}
			)

			var nodes []reftree.Node

			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.StructField && node.StructField.Name == "B1" {
					return reftree.Skip
				}
				return nil
			})

			value.Set(t, reflect.ValueOf(c0))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a1, node.Value.Interface())
			}, "a1 is not visited")
		})

		s.Test("Visitor#Skip will not the interrupt the iteration of the struct fields", func(t *testcase.T) {
			type A struct {
				V int
			}
			type B struct {
				A A
				V string
			}
			type C struct {
				B1 B
				B2 B
				V  float32
			}

			var (
				a1 = A{V: t.Random.Int()}
				a2 = A{V: t.Random.Int()}
				b1 = B{A: a1, V: t.Random.String()}
				b2 = B{A: a2, V: t.Random.String()}
				c0 = C{B1: b1, B2: b2, V: t.Random.Float32()}
			)

			var nodes []reftree.Node

			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.StructField && node.StructField.Name == "B1" {
					return reftree.Skip
				}
				return nil
			})

			value.Set(t, reflect.ValueOf(c0))

			assert.NoError(t, act(t))

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.Struct)
				assert.Equal[any](t, a2, node.Value.Interface())
			}, "but a2 is visited since the iteration should continued")
		})
	})

	s.Context("array", func(s *testcase.Spec) {
		var array = let.Var(s, func(t *testcase.T) [7]string {
			var ary [7]string
			for i, val := range random.Slice(7, t.Random.Domain, random.UniqueValues) {
				ary[i] = val
			}
			return ary
		})

		value.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(array.Get(t))
		})

		s.Then("array itself is visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
				assert.Equal(tb, v.Type, reftree.Array)
				assert.Equal[any](tb, array.Get(t), v.Value.Interface())
			})
		})

		s.Then("elements are visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			for _, sval := range array.Get(t) {
				assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
					assert.Equal(tb, v.Type, reftree.ArrayElem)
					assert.Equal[any](tb, sval, v.Value.Interface())
				})
			}
		})

		s.Then("each element and the array is visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)
			assert.Equal(t, len(vs), len(array.Get(t))+1, "array and the elements were expected to be visited")
		})

		s.When("elements are visitable", func(s *testcase.Spec) {
			var array = let.Var(s, func(t *testcase.T) [7]Visitable {
				var ary [7]Visitable
				for i, val := range random.Slice(7, func() Visitable { return makeVisitable(t) }, random.UniqueValues) {
					ary[i] = val
				}
				return ary
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(array.Get(t))
			})

			s.Then("element is visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, elem := range array.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.Equal(tb, v.Type, reftree.ArrayElem)
						assert.Equal[any](tb, elem, v.Value.Interface())
					})
				}
			})

			s.Then("the elem itself is traversed", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, elem := range array.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.True(tb, v.Is(reftree.ArrayElem))
						assert.Equal[any](tb, elem.V, v.Value.Interface())
					})
				}
			})
		})

		s.Test("Visitor#Break skips the currently visited item AND break the iteration of the map", func(t *testcase.T) {
			var ary = [3]Visitable{
				t.Random.Make(Visitable{}).(Visitable),
				t.Random.Make(Visitable{}).(Visitable),
				t.Random.Make(Visitable{}).(Visitable),
			}

			type W struct {
				A [3]Visitable
				V testent.Foo
			}

			var w = W{
				A: ary,
				V: testent.MakeFoo(t),
			}

			var nodes []reftree.Node
			var breaks int
			visit.Set(t, func(node reftree.Node) error {
				if node.Type == reftree.ArrayElem {
					breaks++
					return reftree.Break
				}
				nodes = append(nodes, node)
				return nil
			})
			value.Set(t, reflect.ValueOf(w))

			assert.NoError(t, act(t))
			assert.Equal(t, breaks, 1)

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, node.Type, reftree.ArrayElem)
			})

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.StructField)
				assert.Equal[any](t, node.Value.Interface(), w.V)
			})
		})
	})

	s.Context("slice", func(s *testcase.Spec) {
		var slc = let.Var(s, func(t *testcase.T) []string {
			return random.Slice(t.Random.IntBetween(1, 3), t.Random.Domain, random.UniqueValues)
		})

		value.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(slc.Get(t))
		})

		s.Then("slice itself is visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
				assert.Equal(tb, v.Type, reftree.Slice)
				assert.Equal[any](tb, slc.Get(t), v.Value.Interface())
			})
		})

		s.Then("elements are visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			for _, sval := range slc.Get(t) {
				assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
					assert.Equal(tb, v.Type, reftree.SliceElem)
					assert.Equal[any](tb, sval, v.Value.Interface())
				})
			}
		})

		s.Then("elements visited by index order", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			var (
				indexes []int
				vals    []string
			)
			for _, n := range vs {
				if n.Type != reftree.SliceElem {
					continue
				}
				indexes = append(indexes, n.Index)
				vals = append(vals, n.Value.String())
			}

			for i, _ := range slc.Get(t) {
				assert.Equal(t, indexes[i], i)
			}

			assert.Equal(t, vals, slc.Get(t))
		})

		s.Then("each element and the slice is visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)
			assert.Equal(t, len(vs), len(slc.Get(t))+1, "slice and the slice elements were expected to be visited")
		})

		s.When("elements are visitable", func(s *testcase.Spec) {
			var slc = let.Var(s, func(t *testcase.T) []Visitable {
				return random.Slice(t.Random.IntBetween(1, 3), func() Visitable {
					return makeVisitable(t)
				}, random.UniqueValues)
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(slc.Get(t))
			})

			s.Then("element is visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, elem := range slc.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.Equal(tb, v.Type, reftree.SliceElem)
						assert.Equal[any](tb, elem, v.Value.Interface())
					})
				}
			})

			s.Then("the elem itself is traversed", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, elem := range slc.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.True(tb, v.Is(reftree.SliceElem))
						assert.Equal[any](tb, elem.V, v.Value.Interface())
					})
				}
			})
		})

		s.Test("Visitor#Skip can stop further down traversing without breaking the iteration", func(t *testcase.T) {
			var slc = []Visitable{
				makeVisitable(t),
				makeVisitable(t),
				makeVisitable(t),
			}

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.SliceElem && node.Index == 1 {
					return reftree.Skip
				}
				return nil
			})
			value.Set(t, reflect.ValueOf(slc))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[1].V, node.Value.Interface())
			})
			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[0].V, node.Value.Interface())
			})
			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[2].V, node.Value.Interface())
			})
		})

		s.Test("Visitor#Skip only skips the current element without breaking the iteration", func(t *testcase.T) {
			var slc = []Visitable{
				makeVisitable(t),
				makeVisitable(t),
				makeVisitable(t),
			}

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.SliceElem && node.Index == 1 {
					return reftree.Skip
				}
				return nil
			})
			value.Set(t, reflect.ValueOf(slc))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[1].V, node.Value.Interface())
			})
			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[0].V, node.Value.Interface())
			})
			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, slc[2].V, node.Value.Interface())
			})
		})

		s.Test("Visitor#Break stops the slice traversing", func(t *testcase.T) {
			slc := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int, random.UniqueValues)

			type W struct {
				S []int
				V int
			}

			var w = W{
				S: slc,
				V: random.Unique(t.Random.Int, slc...),
			}

			var firstElem reftree.Node
			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				if node.Type == reftree.SliceElem { // step out on the first slice elem
					firstElem = node
					return reftree.Break
				}
				nodes = append(nodes, node)
				return nil
			})

			value.Set(t, reflect.ValueOf(w))

			assert.NoError(t, act(t))

			assert.NotNil(t, firstElem)
			assert.NotEmpty(t, firstElem)
			assert.NoneOf(t, nodes, func(t testing.TB, n reftree.Node) {
				assert.Equal(t, n.Type, reftree.SliceElem)
				assert.NotEqual(t, n.Index, firstElem.Index)
			})

			for _, n := range slc {
				elemT := reflect.TypeOf(slc).Elem()
				notExpectedValue := reflect.ValueOf(n)
				assert.NoneOf(t, nodes, func(t testing.TB, n reftree.Node) {
					assert.Equal(t, n.Type, reftree.SliceElem)
					assert.Equal(t, n.Value.Type(), elemT)
					assert.Equal(t, notExpectedValue, n.Value)
				})
			}
		})
	})

	s.Context("map", func(s *testcase.Spec) {
		var m = let.Var(s, func(t *testcase.T) map[string]int {
			return random.Map(t.Random.IntBetween(1, 3), func() (string, int) {
				return t.Random.String(), t.Random.Int()
			}, random.UniqueValues)
		})

		value.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(m.Get(t))
		})

		s.Then("map itself is visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
				assert.Equal(tb, v.Type, reftree.Map)
				assert.Equal[any](tb, m.Get(t), v.Value.Interface())
			})
		})

		s.Then("map's key-values are visited", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			for key, value := range m.Get(t) {
				assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
					assert.Equal(tb, v.Type, reftree.MapKey)
					assert.Equal[any](tb, key, v.Value.Interface())
				})
				assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
					assert.Equal(tb, v.Type, reftree.MapValue)
					assert.Equal[any](tb, value, v.Value.Interface())
				})
			}
		})

		s.When("key has visitable fields", func(s *testcase.Spec) {
			var m = let.Var(s, func(t *testcase.T) map[Visitable]int {
				return random.Map(t.Random.IntBetween(1, 3), func() (Visitable, int) {
					return makeVisitable(t), t.Random.Int()
				}, random.UniqueValues)
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(m.Get(t))
			})

			s.Then("key itself visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for key, _ := range m.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.Equal(tb, v.Type, reftree.MapKey)
						assert.Equal[any](tb, key, v.Value.Interface())
					})
				}
			})

			s.Then("key's values are visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for key, _ := range m.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.True(tb, v.Is(reftree.MapKey))
						assert.Equal[any](tb, key.V, v.Value.Interface())
					})
				}
			})
		})

		s.When("map values are visitable", func(s *testcase.Spec) {
			var m = let.Var(s, func(t *testcase.T) map[int]Visitable {
				return random.Map(t.Random.IntBetween(1, 3), func() (int, Visitable) {
					return t.Random.Int(), makeVisitable(t)
				}, random.UniqueValues)
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(m.Get(t))
			})

			s.Then("key itself visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, mval := range m.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.Equal(tb, v.Type, reftree.MapValue)
						assert.Equal[any](tb, mval, v.Value.Interface())
					})
				}
			})

			s.Then("key's values are visited", func(t *testcase.T) {
				vs, err := collect(t)
				assert.NoError(t, err)

				for _, mval := range m.Get(t) {
					assert.OneOf(t, vs, func(tb testing.TB, v reftree.Node) {
						assert.True(tb, v.Is(reftree.MapValue))
						assert.Equal[any](tb, mval.V, v.Value.Interface())
					})
				}
			})
		})

		s.Test("Visitor#Skip can stop further down traversing", func(t *testcase.T) {
			var m = map[int]Visitable{
				1: makeVisitable(t),
				2: makeVisitable(t),
				3: makeVisitable(t),
			}

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.MapValue && node.MapKey.Int() == 2 {
					return reftree.Skip
				}
				return nil
			})
			value.Set(t, reflect.ValueOf(m))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, m[2].V, node.Value.Interface())
			})

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, m[3].V, node.Value.Interface())
			})
		})

		s.Test("Visitor#Skip skips the currently visited item but doesn't break the iteration of the map", func(t *testcase.T) {
			var m = map[int]Visitable{
				1: makeVisitable(t),
				2: makeVisitable(t),
				3: makeVisitable(t),
			}

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				if node.Type == reftree.MapValue && node.MapKey.Int() == 2 {
					return reftree.Skip
				}
				return nil
			})
			value.Set(t, reflect.ValueOf(m))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, m[2].V, node.Value.Interface())
			})

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, m[3].V, node.Value.Interface())
			})
		})

		s.Test("Visitor#Break skips the currently visited item AND break the iteration of the map", func(t *testcase.T) {
			var m = map[int]Visitable{
				1: makeVisitable(t),
				2: makeVisitable(t),
				3: makeVisitable(t),
			}

			type W struct {
				M map[int]Visitable
				V testent.Foo
			}

			var w = W{
				M: m,
				V: testent.MakeFoo(t),
			}

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				if node.Type == reftree.MapValue {
					return reftree.Break
				}
				nodes = append(nodes, node)
				return nil
			})
			value.Set(t, reflect.ValueOf(w))

			assert.NoError(t, act(t))

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, m[2].V, node.Value.Interface())
			})

			assert.NoneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal[any](t, node.Type, reftree.MapValue)
			})

			assert.OneOf(t, nodes, func(t testing.TB, node reftree.Node) {
				assert.Equal(t, node.Type, reftree.StructField)
				assert.Equal[any](t, node.Value.Interface(), w.V)
			})
		})
	})

	s.Test("pointer", func(t *testcase.T) {
		var input = pointer.Of(t.Random.Int())

		vs, err := collectFrom(t, reflect.ValueOf(input))
		assert.NoError(t, err)

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
		value.Let(s, func(t *testcase.T) reflect.Value {
			var x testent.Fooer = concrete.Get(t)
			rv := reflect.ValueOf(&x).Elem()
			assert.Equal(t, rv.Type(), reflectkit.TypeOf[testent.Fooer]())
			return rv
		})

		s.Then("it will visit the interface, and its value both", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)
			wInterfaceVisitCount := len(vs)

			cVS, err := collectFrom(t, reflect.ValueOf(concrete.Get(t)))
			assert.NoError(t, err)
			wValueVisitCount := len(cVS)
			assert.Equal(t, wInterfaceVisitCount, 1+wValueVisitCount, "interface + interface values")
		})

		s.Then("the visited values will contain the interface node", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

			FooerT := reflectkit.TypeOf[testent.Fooer]()
			assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
				assert.Equal(tb, got.Type, reftree.Interface)
				assert.Equal(tb, got.Value.Kind(), reflect.Interface)
				assert.Equal(tb, got.Value.Type(), FooerT)
				assert.Equal[any](tb, concrete.Get(t), got.Value.Interface())
			})
		})

		s.Then("the visited values will contain the interface elem node", func(t *testcase.T) {
			vs, err := collect(t)
			assert.NoError(t, err)

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
		vs, err := collectFrom(t, rv)
		assert.NoError(t, err)

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

			vvs, err := collectFrom(t, reflect.ValueOf(vs))
			assert.NoError(t, err)

			assert.OneOf(t, vvs, func(t testing.TB, got reftree.Node) {
				assert.True(t, got.Is(reftree.SliceElem))
				assert.Equal(t, got.Value.Type(), elemType)
			})
		})

		s.Test("array elem", func(t *testcase.T) {
			var vs [3]int = [3]int{1, 2, 3}
			elemType := reflectkit.TypeOf[int]()

			vvs, err := collectFrom(t, reflect.ValueOf(vs))
			assert.NoError(t, err)

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

			vvs, err := collectFrom(t, reflect.ValueOf(T{V: t.Random.HexN(4)}))
			assert.NoError(t, err)

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

			vvs, err := collectFrom(t, reflect.ValueOf(&n))
			assert.NoError(t, err)

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

			vvs, err := collectFrom(t, reflect.ValueOf(&i).Elem())
			assert.NoError(t, err)

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

			var nodes []reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				nodes = append(nodes, node)
				return nil
			})
			value.Set(t, reflect.ValueOf(v))
			assert.NoError(t, act(t))

			assert.Equal(t, 3, len(nodes), "pointer -> interface -> int")

			var expectedValueType = reflectkit.TypeOf(n)
			assert.OneOf(t, nodes, func(t testing.TB, got reftree.Node) {
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

			var nodeX reftree.Node
			visit.Set(t, func(node reftree.Node) error {
				if node.Type != reftree.StructField {
					return nil
				}
				if node.StructField.Name == "X" {
					nodeX = node
					return reftree.Stop
				}
				return nil
			})
			value.Set(t, rv)
			assert.NoError(t, act(t))

			nodeX.Value.Set(reflect.ValueOf("foo"))
			assert.Equal(t, v.X, "foo")
		})

		s.Test("setting a field value using the visited reflection value", func(t *testcase.T) {
			var (
				foo              = testent.MakeFoo(t)
				fooFooFieldValue reftree.Node
				found            bool
			)

			visit.Set(t, func(v reftree.Node) error {
				if v.Type != reftree.StructField {
					return nil
				}
				if v.StructField.Name == "Foo" {
					fooFooFieldValue = v
					found = true
					return reftree.Stop
				}
				return nil
			})
			value.Set(t, reflect.ValueOf(&foo))
			assert.NoError(t, act(t))

			assert.True(t, found, assert.MessageF("expected that a %T has a Foo field", foo))

			exp := t.Random.Domain()
			fooFooFieldValue.Value.Set(reflect.ValueOf(exp))
			assert.Equal(t, foo.Foo, exp)
		})
	})

	s.Describe("recursion guarding", func(s *testcase.Spec) {
		s.Context("struct with self referencing field", func(s *testcase.Spec) {
			type T struct {
				V int
				P *T
			}

			vP := let.Var(s, func(t *testcase.T) *T {
				var v T
				v.V = t.Random.Int()
				v.P = &v
				return &v
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(vP.Get(t))
			})

			s.Then("it will visit the values only once", func(t *testcase.T) {
				var vs []reftree.Node

				assert.Within(t, time.Second, func(ctx context.Context) {
					values, err := collect(t)
					assert.NoError(t, err)
					vs = values
				})

				assert.True(t, len(vs) < 10)

				self := *vP.Get(t)

				assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
					assert.Equal(t, got.Type, reftree.Struct)
					assert.Equal[any](t, self, got.Value.Interface())
				})

				assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
					assert.True(t, got.Is(reftree.StructField))
					assert.Equal(t, "V", got.StructField.Name)
					assert.Equal[any](t, self.V, got.Value.Interface())
				})

				assert.OneOf(t, vs, func(t testing.TB, got reftree.Node) {
					assert.True(t, got.Is(reftree.StructField))
					assert.Equal(t, "P", got.StructField.Name)
					gotP, ok := got.Value.Interface().(*T)
					assert.True(t, ok)
					assert.Equal(t, unsafe.Pointer(self.P), unsafe.Pointer(gotP))
				})
			})
		})

		s.Context("mutually referencing structs", func(s *testcase.Spec) {
			aP, bP := let.Var2(s, func(t *testcase.T) (*reftestent.A, *reftestent.B) {
				var (
					a reftestent.A
					b reftestent.B
				)
				a.B = &b
				b.A = &a
				return &a, &b
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(random.Pick(t.Random,
					func() any { return aP.Get(t) },
					func() any { return bP.Get(t) },
				)())
			})

			s.Then("it will visit the values only once and don't fall in inf recursion", func(t *testcase.T) {
				var vs []reftree.Node

				assert.Within(t, time.Second, func(ctx context.Context) {
					values, err := collect(t)
					assert.NoError(t, err)
					vs = values
				})

				assert.True(t, len(vs) < 10)

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *aP.Get(t), got.Value.Interface())
				}, "*A struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *bP.Get(t), got.Value.Interface())
				}, "*B struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "A", got.StructField.Name)
					assert.Equal[any](tb, aP.Get(t), got.Value.Interface())
				})

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "B", got.StructField.Name)
					assert.Equal[any](tb, bP.Get(t), got.Value.Interface())
				})
			})
		})

		s.Context("interface values with circular references", func(s *testcase.Spec) {
			type T struct {
				P any
			}

			aP, bP := let.Var2(s, func(t *testcase.T) (*T, *T) {
				var a, b T
				a.P = &b
				b.P = &a
				return &a, &b
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(random.Pick(t.Random,
					func() any { return aP.Get(t) },
					func() any { return bP.Get(t) },
				)())
			})

			s.Then("it will visit the values only once and don't fall in inf recursion", func(t *testcase.T) {
				var vs []reftree.Node

				assert.Within(t, time.Second, func(ctx context.Context) {
					values, err := collect(t)
					assert.NoError(t, err)
					vs = values
				})

				assert.True(t, len(vs) < 10)

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *aP.Get(t), got.Value.Interface())
				}, "*A struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *bP.Get(t), got.Value.Interface())
				}, "*B struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "P", got.StructField.Name)
					assert.Equal[any](tb, aP.Get(t), got.Value.Interface())
				})

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "P", got.StructField.Name)
					assert.Equal[any](tb, bP.Get(t), got.Value.Interface())
				})
			})
		})

		s.Context("interface values with circular references", func(s *testcase.Spec) {
			type T struct {
				P any
			}

			aP, bP := let.Var2(s, func(t *testcase.T) (*T, *T) {
				var a, b T
				a.P = &b
				b.P = &a
				return &a, &b
			})

			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.ValueOf(random.Pick(t.Random,
					func() any { return aP.Get(t) },
					func() any { return bP.Get(t) },
				)())
			})

			s.Then("it will visit the values only once and don't fall in inf recursion", func(t *testcase.T) {
				var vs []reftree.Node

				assert.Within(t, time.Second, func(ctx context.Context) {
					values, err := collect(t)
					assert.NoError(t, err)
					vs = values
				})

				assert.True(t, len(vs) < 10)

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *aP.Get(t), got.Value.Interface())
				}, "*A struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.Equal(tb, got.Type, reftree.Struct)
					assert.Equal[any](tb, *bP.Get(t), got.Value.Interface())
				}, "*B struct value is visited")

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "P", got.StructField.Name)
					assert.Equal[any](tb, aP.Get(t), got.Value.Interface())
				})

				assert.OneOf(t, vs, func(tb testing.TB, got reftree.Node) {
					assert.True(tb, got.Is(reftree.StructField))
					assert.Equal(tb, "P", got.StructField.Name)
					assert.Equal[any](tb, bP.Get(t), got.Value.Interface())
				})
			})
		})
	})
}

func TestIter_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		v = let.Var[reflect.Value](s, nil)
	)
	act := let.Act(func(t *testcase.T) iter.Seq[reftree.Node] {
		return reftree.Iter(v.Get(t))
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

func TestRecursionGuard(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *reftree.RecursionGuard {
		return &reftree.RecursionGuard{}
	})

	s.Describe("#Seen", func(s *testcase.Spec) {
		var method = func(t *testcase.T, v reflect.Value) bool {
			return subject.Get(t).Seen(v)
		}
		//---
		var (
			value = let.Var[reflect.Value](s, nil)
		)
		act := let.Act(func(t *testcase.T) bool {
			return method(t, value.Get(t))
		})

		var thenSeen = func(s *testcase.Spec) {
			s.Then("it will reported as already seen", func(t *testcase.T) {
				assert.True(t, act(t), "expected to seen the value")
			})
		}

		var thenNotSeen = func(s *testcase.Spec) {
			s.Then("it will reported as not yet seen", func(t *testcase.T) {
				assert.False(t, act(t), "expected to not yet seen the value")
			})
		}

		s.When("value is invalid", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) reflect.Value {
				return reflect.Value{}
			})

			thenNotSeen(s)
		})

		s.When("value is a valid non-addressable value", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) reflect.Value {
				var val = random.Pick(t.Random,
					func() any { return t.Random.String() },
					func() any { return t.Random.Time() },
					func() any { return t.Random.Int() },
					func() any { return t.Random.Float32() },
					func() any { return t.Random.Float64() },
					func() any { return testent.MakeFoo(t) },
					func() any { return random.Slice(t.Random.IntBetween(1, 3), t.Random.Int) },
				)()
				return reflect.ValueOf(val)
			})

			thenNotSeen(s)
		})

		s.When("value is a valid addressable value", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) reflect.Value {
				var val = random.Pick(t.Random,
					func() any { return pointer.Of(t.Random.String()) },
					func() any { return pointer.Of(t.Random.Time()) },
					func() any { return pointer.Of(t.Random.Int()) },
					func() any { return pointer.Of(t.Random.Float32()) },
					func() any { return pointer.Of(t.Random.Float64()) },
					func() any { return pointer.Of(testent.MakeFoo(t)) },
					func() any { return pointer.Of(random.Slice(t.Random.IntBetween(1, 3), t.Random.Int)) },
				)()
				return reflect.ValueOf(val)
			})

			thenNotSeen(s)

			s.And("value was visited already", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					assert.False(t, act(t))
				})

				thenSeen(s)
			})
		})

		s.When("value is a struct that contains a self-reference", func(s *testcase.Spec) {
			type T struct{ P *T }

			value.Let(s, func(t *testcase.T) reflect.Value {
				var v T
				v.P = &v
				return reflect.ValueOf(&v)
			})

			thenNotSeen(s)

			s.Then("checking seen on the pointer value will report seen already", func(t *testcase.T) {
				act(t)

				pField := value.Get(t).Elem().FieldByName("P")
				assert.True(t, method(t, pField))
				assert.False(t, method(t, pField.Elem()),
					"Since we treat values as non recursivable values by default,",
					"and mostly pointers should cause recursion,",
					"it is safe to say that a value by default is not considered seen even,",
					"if it is addressable and its address is already seen")
			})
		})
	})
}
