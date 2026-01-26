package refvis_test

import (
	"iter"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit/refvis"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestNode(t *testing.T) {
	s := testcase.NewSpec(t)

	node := let.Var[refvis.Node](s, nil)

	s.Describe("#Is", func(s *testcase.Spec) {
		var (
			nodeType = let.Var[refvis.NodeType](s, nil)
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
			node.Let(s, func(t *testcase.T) refvis.Node {
				return refvis.Node{}
			})

			s.Context("regardless what node type is asked apart from unknown", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) refvis.NodeType {
					return random.Unique(func() refvis.NodeType {
						return random.Pick(t.Random, NodeTypes...)
					}, refvis.Unknown)
				})

				s.Test("it will be false", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			s.Context("checked for unknown node type", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) refvis.NodeType {
					return refvis.Unknown
				})

				s.Test("it will be true", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})
		})

		s.When("node has the same type as the asked one", func(s *testcase.Spec) {
			nodeType.Let(s, func(t *testcase.T) refvis.NodeType {
				return random.Pick(t.Random, NodeTypes...)
			})

			node.Let(s, func(t *testcase.T) refvis.Node {
				return refvis.Node{
					Type: nodeType.Get(t),
				}
			})

			s.Then("it will report a match", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the node value is an Embedded node type is struct field", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) refvis.Node {
				return refvis.Node{
					Type: refvis.Value,
					Parent: &refvis.Node{
						Type: refvis.PointerElem,
						Parent: &refvis.Node{
							Type: refvis.Pointer,
							Parent: &refvis.Node{
								Type: refvis.StructField,
								Parent: &refvis.Node{
									Type: refvis.Struct,
								},
							},
						},
					},
				}
			})

			s.And("an embedding/container node type is asked", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) refvis.NodeType {
					return random.Pick(t.Random, refvis.StructField, refvis.PointerElem)
				})

				s.Then("it is reported to be true", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})

			s.And("a concrete value type from the top of the node parent chain is asked", func(s *testcase.Spec) {
				nodeType.LetValue(s, refvis.Struct)

				s.Then("it is reported as false, because the current node itself is only contained within but not the requested node type", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			s.And("different node type is asked", func(s *testcase.Spec) {
				nodeType.Let(s, func(t *testcase.T) refvis.NodeType {
					return random.Pick(t.Random, refvis.ArrayElem, refvis.SliceElem, refvis.MapValue)
				})

				s.Then("it is reported to be false", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		})
	})

	s.Describe("#Iter", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[refvis.Node] {
			return node.Get(t).Iter()
		})

		s.When("node doesn't have a parent", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) refvis.Node {
				return random.Pick(t.Random,
					refvis.Node{},
					refvis.Node{Type: refvis.Value, Value: reflect.ValueOf(t.Random.String())},
				)
			})

			s.Then("it will only yield itself", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []refvis.Node{node.Get(t)})
			})
		})

		s.When("node has parent(s)", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) refvis.Node {
				t.Log("Given that Node is a child of a child element on a reflection node tree.")
				return refvis.Node{
					Type: refvis.Value,
					Parent: &refvis.Node{
						Type: refvis.Pointer,
						Parent: &refvis.Node{
							Type: refvis.StructField,
							Parent: &refvis.Node{
								Type: refvis.Struct,
							},
						},
					},
				}
			})

			s.Then("iteration will go through and yield elements from the outer towards the inner values", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []refvis.Node{
					*node.Get(t).Parent.Parent.Parent,
					*node.Get(t).Parent.Parent,
					*node.Get(t).Parent,
					node.Get(t),
				})
			})
		})
	})

	s.Describe("#IterUpward", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[refvis.Node] {
			return node.Get(t).IterUpward()
		})

		s.When("node doesn't have a parent", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) refvis.Node {
				return random.Pick(t.Random,
					refvis.Node{},
					refvis.Node{Type: refvis.Value, Value: reflect.ValueOf(t.Random.String())},
				)
			})

			s.Then("it will only yield itself", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []refvis.Node{node.Get(t)})
			})
		})

		s.When("node has parent(s)", func(s *testcase.Spec) {
			node.Let(s, func(t *testcase.T) refvis.Node {
				t.Log("Given that Node is a child of a child element on a reflection node tree.")
				return refvis.Node{
					Type: refvis.Value,
					Parent: &refvis.Node{
						Type: refvis.Pointer,
						Parent: &refvis.Node{
							Type: refvis.StructField,
							Parent: &refvis.Node{
								Type: refvis.Struct,
							},
						},
					},
				}
			})

			s.Then("iteration will go through and yield elements from the current node, and sequentially each outer parent", func(t *testcase.T) {
				vs := iterkit.Collect(act(t))

				assert.Equal(t, vs, []refvis.Node{
					node.Get(t),
					*node.Get(t).Parent,
					*node.Get(t).Parent.Parent,
					*node.Get(t).Parent.Parent.Parent,
				})
			})
		})
	})
}

var NodeTypes = []refvis.NodeType{
	refvis.Unknown,
	refvis.Value,
	refvis.Struct,
	refvis.StructField,
	refvis.Array,
	refvis.ArrayElem,
	refvis.Slice,
	refvis.SliceElem,
	refvis.Interface,
	refvis.InterfaceElem,
	refvis.Pointer,
	refvis.PointerElem,
	refvis.Map,
	refvis.MapKey,
	refvis.MapValue,
}

func TestPath_Contains(t *testing.T) {
	t.Run("empty ntp returns true", func(t *testing.T) {
		p := refvis.Path{}
		assert.True(t, p.Contains())
	})

	t.Run("ntp matches exactly path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp shorter than path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp missing an element from fully being contained 1:1 in the Path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("ntp longer than path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("some elements mismatch", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.PointerElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("p empty and ntp non-empty", func(t *testing.T) {
		p := refvis.Path{}
		ntp := []refvis.NodeType{refvis.Struct}
		assert.False(t, p.Contains(ntp...))
	})
}
