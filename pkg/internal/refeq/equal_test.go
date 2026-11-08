package refeq_test

import (
	"reflect"
	"runtime"
	"testing"

	"go.llib.dev/frameless/pkg/internal/refeq"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

// Order nests Customer as its first field,
// so an Order, its Customer and the Customer's ID all start at the same address.
type Order struct {
	Customer Customer
	Note     string
}

type Customer struct{ ID string }

type Node struct {
	V    string
	Next *Node
}

// ring makes a cyclic list, where the last node points back to the first one.
func ring(vs ...string) *Node {
	head := &Node{V: vs[0]}
	last := head
	for _, v := range vs[1:] {
		last.Next = &Node{V: v}
		last = last.Next
	}
	last.Next = head
	return head
}

// GCTrigger runs the garbage collector each time it is compared,
// so the memory which the comparison released before it is reused by the allocations after it.
type GCTrigger struct{}

func (GCTrigger) Equal(GCTrigger) bool {
	runtime.GC()
	return true
}

type Item struct {
	GC GCTrigger
	V  string
}

func TestEqual(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		id   = let.StringNC(s, 8, random.CharsetAlpha())
		note = let.StringNC(s, 8, random.CharsetAlpha())
		v1   = let.Var(s, func(t *testcase.T) any {
			return []Order{{Customer: Customer{ID: id.Get(t)}, Note: note.Get(t)}}
		})
		v2 = let.Var(s, func(t *testcase.T) any {
			return []Order{{Customer: Customer{ID: id.Get(t)}, Note: note.Get(t)}}
		})
	)
	act := func(t *testcase.T) bool {
		return refeq.Equal(reflect.ValueOf(v1.Get(t)), reflect.ValueOf(v2.Get(t)))
	}

	s.Then("values with the same content are equal", func(t *testcase.T) {
		assert.True(t, act(t))
	})

	s.When("they differ in the first field of a nested struct", func(s *testcase.Spec) {
		othID := let.Var(s, func(t *testcase.T) string {
			return random.Unique(func() string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			}, id.Get(t))
		})
		v2.Let(s, func(t *testcase.T) any {
			return []Order{{Customer: Customer{ID: othID.Get(t)}, Note: note.Get(t)}}
		})

		s.Then("they are not equal", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("one of them references the same value at two places", func(s *testcase.Spec) {
		var (
			a = let.StringNC(s, 8, random.CharsetAlpha())
			b = let.Var(s, func(t *testcase.T) string {
				return random.Unique(func() string {
					return t.Random.StringNC(8, random.CharsetAlpha())
				}, a.Get(t))
			})
		)
		v1.Let(s, func(t *testcase.T) any {
			shared := &Node{V: b.Get(t)}
			return []*Node{{V: a.Get(t)}, shared, shared}
		})

		s.And("the other one has a different value at the second place", func(s *testcase.Spec) {
			v2.Let(s, func(t *testcase.T) any {
				return []*Node{{V: a.Get(t)}, {V: b.Get(t)}, {V: a.Get(t)}}
			})

			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.And("the other one has values with the same content at both places", func(s *testcase.Spec) {
			v2.Let(s, func(t *testcase.T) any {
				return []*Node{{V: a.Get(t)}, {V: b.Get(t)}, {V: b.Get(t)}}
			})

			s.Then("they are equal", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})

	s.When("they are self-referencing", func(s *testcase.Spec) {
		a := let.StringNC(s, 8, random.CharsetAlpha())
		v1.Let(s, func(t *testcase.T) any { return ring(a.Get(t)) })
		v2.Let(s, func(t *testcase.T) any { return ring(a.Get(t)) })

		s.Then("the comparison terminates, and finds them equal", func(t *testcase.T) {
			assert.True(t, act(t))
		})

		s.And("the cycle of the other one is longer, with different content", func(s *testcase.Spec) {
			b := let.Var(s, func(t *testcase.T) string {
				return random.Unique(func() string {
					return t.Random.StringNC(8, random.CharsetAlpha())
				}, a.Get(t))
			})
			v2.Let(s, func(t *testcase.T) any { return ring(a.Get(t), b.Get(t)) })

			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.When("the garbage collector runs while they are compared", func(s *testcase.Spec) {
		items := let.Var(s, func(t *testcase.T) [4]Item {
			var items [4]Item
			for i := range items {
				items[i] = Item{V: t.Random.StringNC(8, random.CharsetAlpha())}
			}
			return items
		})
		v1.Let(s, func(t *testcase.T) any { return items.Get(t) })
		v2.Let(s, func(t *testcase.T) any {
			oth := items.Get(t)
			last := len(oth) - 1
			oth[last].V = random.Unique(func() string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			}, oth[last].V)
			return oth
		})

		s.Then("a difference after the collection is still found", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})
}
