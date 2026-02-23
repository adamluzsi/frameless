package dscontract_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/frameless/port/ds/dslist"
	"go.llib.dev/frameless/port/ds/dsset"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func TestLenAppendable(t *testing.T) {
	s := testcase.NewSpec(t)

	lc := dscontract.ListConfig[string]{
		MakeElem: MakeUniqElem[string](),
	}

	s.Context("Set", dscontract.LenAppendable(func(tb testing.TB) dscontract.SubjectLenAppendable[string] {
		return &dsset.Set[string]{}
	}, lc).Spec)

	s.Context("OrderedSet", dscontract.LenAppendable(func(tb testing.TB) dscontract.SubjectLenAppendable[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Spec)

	s.Context("Slice", dscontract.LenAppendable(func(tb testing.TB) dscontract.SubjectLenAppendable[string] {
		return &dslist.Slice[string]{}
	}, lc).Spec)

	s.Context("LinkedList", dscontract.LenAppendable(func(tb testing.TB) dscontract.SubjectLenAppendable[string] {
		return &dslist.LinkedList[string]{}
	}, lc).Spec)
}

func TestList(t *testing.T) {
	s := testcase.NewSpec(t)

	lc := dscontract.ListConfig[string]{
		MakeElem: MakeUniqElem[string](),
	}

	s.Context("Set", dscontract.List(func(tb testing.TB) ds.List[string] {
		return &dsset.Set[string]{}
	}, lc).Spec)

	s.Context("OrderedSet", dscontract.List(func(tb testing.TB) ds.List[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Spec)

	s.Context("LinkedList", dscontract.List(func(tb testing.TB) ds.List[string] {
		return &dslist.LinkedList[string]{}
	}, lc).Spec)
}

func TestOrderedList(t *testing.T) {
	s := testcase.NewSpec(t)

	lc := dscontract.ListConfig[string]{
		MakeElem: MakeUniqElem[string](),
	}

	s.Context("OrderedSet", dscontract.OrderedList(func(tb testing.TB) ds.List[string] {
		return &dsset.OrderedSet[string]{}
	}, lc).Spec)

	s.Context("LinkedList", dscontract.OrderedList(func(tb testing.TB) ds.List[string] {
		return &dslist.LinkedList[string]{}
	}, lc).Spec)
}

func MakeUniqElem[T any]() func(testing.TB) T {
	vs := make([]T, 0)
	return func(tb testing.TB) T {
		t := testcase.ToT(&tb)
		mk := func() T { return t.Random.Make(reflectkit.TypeOf[T]()).(T) }
		v := random.Unique(mk, vs...)
		vs = append(vs, v)
		return v
	}
}
