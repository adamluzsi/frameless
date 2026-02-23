package reftree_test

import (
	"iter"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestNode(t *testing.T) {
	s := testcase.NewSpec(t)

	node := let.Var[reftree.Node](s, nil)

	s.Describe("#Is", func(s *testcase.Spec) {
		var (
			nodeType = let.Var[reftree.NodeType](s, nil)
		)
		act := let.Act(func(t *testcase.T) bool {
			return node.Get(t).Is(nodeType.Get(t))
		})

		s.Before(func(t *testcase.T) {
			t.OnFail(func() {
				t.Log("node type:", nodeType.Get(t).String())
			})
		})

		s.When("node is a zero value", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				return reftree.Node{}
			})

			s.Context("regardless what node type is asked apart from unknown", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) reftree.NodeType {
					return random.Unique(func() reftree.NodeType {
						return random.Pick(t.Random, NodeTypes...)
					}, reftree.Unknown)
				})

				s.Test("it will be false", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			s.Context("checked for unknown node type", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) reftree.NodeType {
					return reftree.Unknown
				})

				s.Test("it will be true", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})
		})

		s.When("node has the same type as the asked one", func(s *testcase.Spec) {
			nodeType.Let(s, func(t *testcase.T) reftree.NodeType {
				return random.Pick(t.Random, NodeTypes...)
			})

			node.Let(s, func(t *testcase.T) reftree.Node {
				return reftree.Node{
					Type: nodeType.Get(t),
				}
			})

			s.Then("it will report a match", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the node value is an Embedded node type is struct field", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				return reftree.Node{
					Type: reftree.Value,
					Parent: &reftree.Node{
						Type: reftree.PointerElem,
						Parent: &reftree.Node{
							Type: reftree.Pointer,
							Parent: &reftree.Node{
								Type: reftree.StructField,
								Parent: &reftree.Node{
									Type: reftree.Struct,
								},
							},
						},
					},
				}
			})

			s.And("an embedding/container node type is asked", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) reftree.NodeType {
					return random.Pick(t.Random, reftree.StructField, reftree.PointerElem)
				})

				s.Then("it is reported to be true", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})

			s.And("a concrete value type from the top of the node parent chain is asked", func(s *testcase.Spec) {
				nodeType.LetValue(s, reftree.Struct)

				s.Then("it is reported as false, because the current node itself is only contained within but not the requested node type", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			s.And("different node type is asked", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) reftree.NodeType {
					return random.Pick(t.Random, reftree.ArrayElem, reftree.SliceElem, reftree.MapValue)
				})

				s.Then("it is reported to be false", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})
	})

	s.Describe("#Values", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[reftree.Node] {
			return node.Get(t).Iter()
		})

		s.When("node doesn't have a parent", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				return random.Pick(t.Random,
					reftree.Node{},
					reftree.Node{Type: reftree.Value, Value: reflect.ValueOf(t.Random.String())},
				)
			})

			s.Then("it will only yield itself", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []reftree.Node{node.Get(t)})
			})
		})

		s.When("node has parent(s)", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				t.Log("Given that Node is a child of a child element on a reflection node tree.")
				return reftree.Node{
					Type: reftree.Value,
					Parent: &reftree.Node{
						Type: reftree.Pointer,
						Parent: &reftree.Node{
							Type: reftree.StructField,
							Parent: &reftree.Node{
								Type: reftree.Struct,
							},
						},
					},
				}
			})

			s.Then("iteration will go through and yield elements from the outer towards the inner values", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []reftree.Node{
					*node.Get(t).Parent.Parent.Parent,
					*node.Get(t).Parent.Parent,
					*node.Get(t).Parent,
					node.Get(t),
				})
			})
		})
	})

	s.Describe("#ValuesUpward", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[reftree.Node] {
			return node.Get(t).IterUpward()
		})

		s.When("node doesn't have a parent", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				return random.Pick(t.Random,
					reftree.Node{},
					reftree.Node{Type: reftree.Value, Value: reflect.ValueOf(t.Random.String())},
				)
			})

			s.Then("it will only yield itself", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []reftree.Node{node.Get(t)})
			})
		})

		s.When("node has parent(s)", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) reftree.Node {
				t.Log("Given that Node is a child of a child element on a reflection node tree.")
				return reftree.Node{
					Type: reftree.Value,
					Parent: &reftree.Node{
						Type: reftree.Pointer,
						Parent: &reftree.Node{
							Type: reftree.StructField,
							Parent: &reftree.Node{
								Type: reftree.Struct,
							},
						},
					},
				}
			})

			s.Then("iteration will go through and yield elements from the current node, and sequentially each outer parent", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []reftree.Node{
					node.Get(t),
					*node.Get(t).Parent,
					*node.Get(t).Parent.Parent,
					*node.Get(t).Parent.Parent.Parent,
				})
			})
		})
	})
}

var NodeTypes = []reftree.NodeType{
	reftree.Unknown,
	reftree.Value,
	reftree.Struct,
	reftree.StructField,
	reftree.Array,
	reftree.ArrayElem,
	reftree.Slice,
	reftree.SliceElem,
	reftree.Interface,
	reftree.InterfaceElem,
	reftree.Pointer,
	reftree.PointerElem,
	reftree.Map,
	reftree.MapKey,
	reftree.MapValue,
}

func TestPath_Contains(t *testing.T) {
	t.Run("empty ntp returns true", func(t *testing.T) {
		p := reftree.Path{}
		assert.True(t, p.Contains())
	})

	t.Run("ntp matches exactly path", func(t *testing.T) {
		p := reftree.Path{reftree.Array, reftree.ArrayElem}
		ntp := []reftree.NodeType{reftree.Array, reftree.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp shorter than path", func(t *testing.T) {
		p := reftree.Path{reftree.Array, reftree.ArrayElem, reftree.Interface, reftree.InterfaceElem}
		ntp := []reftree.NodeType{reftree.Array, reftree.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp missing an element from fully being contained 1:1 in the Path", func(t *testing.T) {
		p := reftree.Path{reftree.Array, reftree.ArrayElem, reftree.Interface, reftree.InterfaceElem}
		ntp := []reftree.NodeType{reftree.Array, reftree.ArrayElem, reftree.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("ntp longer than path", func(t *testing.T) {
		p := reftree.Path{reftree.Array, reftree.ArrayElem}
		ntp := []reftree.NodeType{reftree.Array, reftree.ArrayElem, reftree.Interface, reftree.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("some elements mismatch", func(t *testing.T) {
		p := reftree.Path{reftree.Array, reftree.ArrayElem, reftree.Interface, reftree.InterfaceElem}
		ntp := []reftree.NodeType{reftree.Array, reftree.ArrayElem, reftree.Interface, reftree.PointerElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("p empty and ntp non-empty", func(t *testing.T) {
		p := reftree.Path{}
		ntp := []reftree.NodeType{reftree.Struct}
		assert.False(t, p.Contains(ntp...))
	})
}
