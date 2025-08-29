package datastructcontract

import (
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/datastruct"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func OrderedList[T any](make func(tb testing.TB) datastruct.List[T], opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	List(make, c).Spec(s)

	s.Test("ordered", func(t *testcase.T) {
		var (
			list         = make(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeT(t) }, random.UniqueValues)
		)
		list.Append(expected...)
		if ts, ok := list.(datastruct.Slicer[T]); ok {
			assert.Equal(t, expected, ts.Slice())
		}
		assert.Equal(t, expected, iterkit.Collect(list.Iter()))
	})

	return s.AsSuite(fmt.Sprintf("ordered List[%s]", reflectkit.TypeOf[T]().String()))
}

func List[T any](make func(tb testing.TB) datastruct.List[T], opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("smoke", func(t *testcase.T) {
		var (
			list         = make(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeT(t) })
		)

		list.Append()
		assert.Equal(t, 0, list.Len())

		var expLen int
		for _, v := range expected {
			assert.Equal(t, expLen, list.Len())
			list.Append(v)
			expLen++
		}

		assert.ContainsExactly(t, expected, iterkit.Collect(list.Iter()))

		if cts, ok := list.(datastruct.Slicer[T]); ok {
			assert.ContainsExactly(t, expected, cts.Slice())
		}
	})

	s.Test("Append many", func(t *testcase.T) {
		var (
			list         = make(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeT(t) })
		)
		list.Append(expected...)
		assert.Equal(t, len(expected), list.Len())
		assert.ContainsExactly(t, expected, iterkit.Collect(list.Iter()))

		if cts, ok := list.(datastruct.Slicer[T]); ok {
			assert.ContainsExactly(t, expected, cts.Slice())
		}
	})

	s.Describe("#Iter", iterkitcontract.IterSeq(func(tb testing.TB) iter.Seq[T] {
		t := testcase.ToT(&tb)
		list := make(t)
		t.Random.Repeat(3, 7, func() {
			v := c.makeT(t)
			list.Append(v)
		})
		return list.Iter()
	}).Spec)

	return s.AsSuite(fmt.Sprintf("List[%s]", reflectkit.TypeOf[T]().String()))
}

type ListOption[T any] interface {
	option.Option[ListConfig[T]]
}

type ListConfig[T any] struct {
	MakeElem func(testing.TB) T
}

var _ ListOption[any] = ListConfig[any]{}

func (c ListConfig[T]) Configure(o *ListConfig[T]) {
	o.MakeElem = zerokit.Coalesce(c.MakeElem, o.MakeElem)
}

func (c ListConfig[T]) makeT(tb testing.TB) T {
	return zerokit.Coalesce(c.MakeElem, spechelper.MakeValue[T])(tb)
}
