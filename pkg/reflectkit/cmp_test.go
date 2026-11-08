package reflectkit_test

import (
	"fmt"
	"net/netip"
	"reflect"
	"runtime"
	"testing"

	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleEqual() {
	reflectkit.Equal("a", "a") // true
	reflectkit.Equal("a", "b") // false
}

func TestEqual(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		v1 = let.Var[any](s, nil)
		v2 = let.Var[any](s, nil)
	)
	act := func(t *testcase.T) bool {
		return reflectkit.Equal(v1.Get(t), v2.Get(t))
	}

	rnd := random.New(random.CryptoSeed{})
	tt := []struct {
		desc    string
		v1, v2  any
		isEqual bool
	}{
		{
			desc:    "equal integers",
			v1:      1,
			v2:      1,
			isEqual: true,
		},
		{
			desc:    "different integers",
			v1:      1,
			v2:      2,
			isEqual: false,
		},
		{
			desc:    "equal strings",
			v1:      "test",
			v2:      "test",
			isEqual: true,
		},
		{
			desc:    "different strings",
			v1:      "test",
			v2:      "test1",
			isEqual: false,
		},
		{
			desc:    "equal slices",
			v1:      []int{1, 2, 3},
			v2:      []int{1, 2, 3},
			isEqual: true,
		},
		{
			desc:    "different slices",
			v1:      []int{1, 2, 3},
			v2:      []int{1, 2, 4},
			isEqual: false,
		},
		{
			desc:    "equal arrays",
			v1:      [3]int{1, 2, 3},
			v2:      [3]int{1, 2, 3},
			isEqual: true,
		},
		{
			desc:    "different arrays",
			v1:      [3]int{1, 2, 3},
			v2:      [3]int{1, 2, 4},
			isEqual: false,
		},
		{
			desc:    "equal maps",
			v1:      map[string]int{"one": 1, "two": 2},
			v2:      map[string]int{"one": 1, "two": 2},
			isEqual: true,
		},
		{
			desc:    "different maps",
			v1:      map[string]int{"one": 1, "two": 2},
			v2:      map[string]int{"one": 1, "two": 3},
			isEqual: false,
		},
		{
			desc:    "equal structs",
			v1:      TestStruct{Field1: 1, Field2: "test"},
			v2:      TestStruct{Field1: 1, Field2: "test"},
			isEqual: true,
		},
		{
			desc:    "different structs",
			v1:      TestStruct{Field1: 1, Field2: "test"},
			v2:      TestStruct{Field1: 2, Field2: "test"},
			isEqual: false,
		},
		{
			desc: "different structs with equality support - equal",
			v1: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support - not equal",
			v1: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with equality support (ptr receiver) - equal",
			v1: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support (ptr receiver) - not equal",
			v1: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with equality support (IsEqual) - equal",
			v1: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with equality support (IsEqual) - not equal",
			v1: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructEquatableWithIsEqual{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with comparable support - equal",
			v1: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with comparable support - not equal",
			v1: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructComparable{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{
			desc: "different structs with comparable support (ptr receiver) - equal",
			v1: TestStructComparableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			v2: TestStructComparableOnPtr{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: true,
		},
		{
			desc: "different structs with comparable support (ptr receiver) - not equal",
			v1: TestStructComparablePointers{
				Irrelevant: rnd.Int(),
				Relevant:   24,
			},
			v2: TestStructComparablePointers{
				Irrelevant: rnd.Int(),
				Relevant:   42,
			},
			isEqual: false,
		},
		{ // netip is a struct that is expected to be comparable with `==``
			desc:    "structs with unexported fields - equal",
			v1:      must.Must(netip.ParsePrefix("10.0.0.0/24")),
			v2:      must.Must(netip.ParsePrefix("10.0.0.0/24")),
			isEqual: true,
		},
		{
			desc:    "structs with unexported fields - equal",
			v1:      must.Must(netip.ParsePrefix("10.0.0.0/24")),
			v2:      must.Must(netip.ParsePrefix("10.0.0.1/24")),
			isEqual: false,
		},
		{
			desc:    "structs with non comparable field - equal",
			v1:      NonComparableStruct{Field1: []int{1, 2, 3}},
			v2:      NonComparableStruct{Field1: []int{1, 2, 3}},
			isEqual: true,
		},
		{
			desc:    "structs with non comparable field - not equal",
			v1:      NonComparableStruct{Field1: []int{1, 2, 3}},
			v2:      NonComparableStruct{Field1: []int{3, 2, 1}},
			isEqual: false,
		},
		{
			desc:    "comparable values under an interfaces such as reflect.Type - equal",
			v1:      reflect.TypeOf((*string)(nil)).Elem(),
			v2:      reflect.TypeOf((*string)(nil)).Elem(),
			isEqual: true,
		},
		{
			desc:    "comparable values under an interfaces such as reflect.Type - not equal",
			v1:      reflect.TypeOf((*string)(nil)).Elem(),
			v2:      reflect.TypeOf((*int)(nil)).Elem(),
			isEqual: false,
		},
	}
	for _, tc := range tt {
		s.Context(tc.desc, func(s *testcase.Spec) {
			v1.Let(s, func(t *testcase.T) any { return tc.v1 })
			v2.Let(s, func(t *testcase.T) any { return tc.v2 })

			s.Then(fmt.Sprintf("Equal returns %t", tc.isEqual), func(t *testcase.T) {
				assert.Equal(t, tc.isEqual, act(t))
			})
		})
	}

	s.Context("structs nested as the first field of other structs", func(s *testcase.Spec) {
		// Order nests Customer as its first field,
		// so an Order, its Customer and the Customer's ID all start at the same address.
		type Customer struct{ ID string }
		type Order struct {
			Customer Customer
			Note     string
		}
		var (
			id   = let.StringNC(s, 8, random.CharsetAlpha())
			note = let.StringNC(s, 8, random.CharsetAlpha())
		)
		v1.Let(s, func(t *testcase.T) any {
			return []Order{{Customer: Customer{ID: id.Get(t)}, Note: note.Get(t)}}
		})
		v2.Let(s, func(t *testcase.T) any {
			return []Order{{Customer: Customer{ID: id.Get(t)}, Note: note.Get(t)}}
		})

		s.Then("values with the same content are equal", func(t *testcase.T) {
			assert.True(t, act(t))
		})

		s.And("they differ only in the first field of the nested struct", func(s *testcase.Spec) {
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
	})

	type Node struct {
		V    string
		Next *Node
	}

	s.When("one value references the same pointer at two places", func(s *testcase.Spec) {
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

		s.And("the other value has a different content at one of those places", func(s *testcase.Spec) {
			v2.Let(s, func(t *testcase.T) any {
				return []*Node{{V: a.Get(t)}, {V: b.Get(t)}, {V: a.Get(t)}}
			})

			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.And("the other value has the same content at both places", func(s *testcase.Spec) {
			v2.Let(s, func(t *testcase.T) any {
				return []*Node{{V: a.Get(t)}, {V: b.Get(t)}, {V: b.Get(t)}}
			})

			s.Then("they are equal", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})

	s.When("the values are self-referencing", func(s *testcase.Spec) {
		// ring makes a cyclic list, where the last node points back to the first one.
		ring := func(vs ...string) *Node {
			head := &Node{V: vs[0]}
			last := head
			for _, v := range vs[1:] {
				last.Next = &Node{V: v}
				last = last.Next
			}
			last.Next = head
			return head
		}
		a := let.StringNC(s, 8, random.CharsetAlpha())
		v1.Let(s, func(t *testcase.T) any { return ring(a.Get(t)) })
		v2.Let(s, func(t *testcase.T) any { return ring(a.Get(t)) })

		s.Then("the comparison terminates, and finds them equal", func(t *testcase.T) {
			assert.True(t, act(t))
		})

		s.And("the cycle of the other value is longer, with a different content", func(s *testcase.Spec) {
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

	s.When("the garbage collector runs while the values are compared", func(s *testcase.Spec) {
		type Item struct {
			GC GCTrigger
			V  string
		}
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

// GCTrigger runs the garbage collector each time it is compared,
// so the memory which the comparison released before it is reused by the allocations after it.
type GCTrigger struct{}

func (GCTrigger) Equal(GCTrigger) bool {
	runtime.GC()
	return true
}

type NonComparableStruct struct {
	Field1 []int
}

type TestStruct struct {
	Field1 int
	Field2 string
}

type TestStructEquatable struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructEquatable) Equal(oth TestStructEquatable) bool {
	return es.Relevant == oth.Relevant
}

type TestStructEquatableOnPtr struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructEquatableOnPtr) Equal(oth TestStructEquatableOnPtr) bool {
	return es.Relevant == oth.Relevant
}

type TestStructEquatableWithIsEqual struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructEquatableWithIsEqual) IsEqual(oth TestStructEquatableWithIsEqual) bool {
	return es.Relevant == oth.Relevant
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a == b:
		return 0
	case a > b:
		return 1
	default:
		panic("unknown Cmp case")
	}
}

type TestStructComparable struct {
	Irrelevant int
	Relevant   int
}

func (es TestStructComparable) Cmp(v TestStructComparable) int {
	return cmp(es.Relevant, v.Relevant)
}

type TestStructComparableOnPtr struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructComparableOnPtr) Cmp(v TestStructComparableOnPtr) int {
	return cmp(es.Relevant, v.Relevant)
}

type TestStructComparablePointers struct {
	Irrelevant int
	Relevant   int
}

func (es *TestStructComparablePointers) Cmp(v *TestStructComparablePointers) int {
	return cmp(es.Relevant, v.Relevant)
}
