package dscontract

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
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dslist"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type SubjectLenAppendable[T any] interface {
	ds.Appendable[T]
	ds.Len
}

func LenAppendable[T any, Subject SubjectLenAppendable[T]](mk func(tb testing.TB) Subject, opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("append affects length", func(t *testcase.T) {
		subject := mk(t)

		exp := 0
		assert.Equal(t, exp, subject.Len())

		t.Random.Repeat(3, 7, func() {
			subject.Append(c.makeElem(t))
			exp++
			assert.Equal(t, exp, subject.Len())
		})
	})

	s.Test("append many at once increase the length by the sum of appended values", func(t *testcase.T) {
		var (
			list         = mk(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeElem(t) })
		)
		baseLen := list.Len()
		list.Append(expected...)
		assert.Equal(t, len(expected)+baseLen, list.Len())
	})

	return s.AsSuite(fmt.Sprintf("Len[%s] (appendable)", reflectkit.TypeOf[T]().String()))
}

func OrderedList[T any, Subject ds.List[T]](mk func(tb testing.TB) Subject, opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	List(mk, c).Spec(s)

	s.Test("ordered", func(t *testcase.T) {
		var (
			list         = mk(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeElem(t) }, random.UniqueValues)
		)
		list.Append(expected...)
		if ts, ok := any(list).(ds.SliceConveratble[T]); ok {
			assert.Equal(t, expected, ts.ToSlice())
		}
		assert.Equal(t, expected, iterkit.Collect(list.Values()))
	})

	return s.AsSuite(fmt.Sprintf("ordered List[%s]", reflectkit.TypeOf[T]().String()))
}

func List[T any, Subject ds.List[T]](mk func(tb testing.TB) Subject, opts ...ListOption[T]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("smoke", func(t *testcase.T) {
		var (
			list         = mk(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeElem(t) })
		)

		list.Append()
		assert.Equal(t, 0, dslist.Len(list))

		var expLen int
		for _, v := range expected {
			assert.Equal(t, expLen, dslist.Len(list))
			list.Append(v)
			expLen++
		}

		assert.ContainsExactly(t, expected, iterkit.Collect(list.Values()))
	})

	s.Test("Append many", func(t *testcase.T) {
		var (
			list         = mk(t)
			expected []T = random.Slice(t.Random.IntBetween(3, 7), func() T { return c.makeElem(t) })
		)
		list.Append(expected...)
		assert.Equal(t, len(expected), dslist.Len(list))
		assert.ContainsExactly(t, expected, iterkit.Collect(list.Values()))

		if cts, ok := any(list).(ds.SliceConveratble[T]); ok {
			assert.ContainsExactly(t, expected, cts.ToSlice())
		}
	})

	s.Describe("#Values", iterkitcontract.IterSeq(func(tb testing.TB) iter.Seq[T] {
		t := testcase.ToT(&tb)
		list := mk(t)
		t.Random.Repeat(3, 7, func() {
			v := c.makeElem(t)
			list.Append(v)
		})
		return list.Values()
	}).Spec)

	s.Context("implements Appendable", Appendable(mk).Spec)

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

func (c ListConfig[T]) makeElem(tb testing.TB) T {
	return zerokit.Coalesce(c.MakeElem, spechelper.MakeValue[T])(tb)
}
