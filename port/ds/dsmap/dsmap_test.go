package dsmap_test

import (
	"iter"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/frameless/port/ds/dsmap"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var _ ds.Map[string, int] = (dsmap.Map[string, int])(nil)

var _ ds.Keys[string] = (dsmap.Map[string, int])(nil)

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)

	m := let.Var(s, func(t *testcase.T) dsmap.Map[string, int] {
		return dsmap.Map[string, int]{}
	})

	s.Test("smoke", func(t *testcase.T) {
		var (
			key  string = t.Random.String()
			val1 int    = t.Random.Int()
			val2 int    = t.Random.Int()
		)

		_, ok := m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, iterkit.Collect(m.Get(t).Keys()))
		assert.Equal(t, 0, m.Get(t).Len())

		m.Get(t).Set(key, val1)
		got, ok := m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val1, got)
		assert.Equal(t, val1, m.Get(t).Get(key))
		assert.Contains(t, iterkit.Collect(m.Get(t).Keys()), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Set(key, val2)
		got, ok = m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val2, got)
		assert.Equal(t, val2, m.Get(t).Get(key))
		assert.Contains(t, iterkit.Collect(m.Get(t).Keys()), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Delete(key)
		_, ok = m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, iterkit.Collect(m.Get(t).Keys()))
		assert.Equal(t, 0, m.Get(t).Len())
	})

	s.Test("#ToMap", func(t *testcase.T) {
		exp := map[string]int{}
		m := dsmap.Map[string, int]{}
		t.Random.Repeat(3, 7, func() {
			k := t.Random.HexN(5)
			v := t.Random.Int()
			exp[k] = v
			m.Set(k, v)
		})
		assert.Equal(t, exp, m.Map())
	})

	s.Context("implements Key-Value-Store", dscontract.Map(func(tb testing.TB) ds.Map[string, int] {
		return m.Get(testcase.ToT(&tb))
	}).Spec)
}

func TestLen(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("ds.Sizer", func(t *testcase.T) {
		exp := t.Random.Int()
		assert.Equal(t, dsmap.Len(KVSSizer{Size: exp}), exp)
	})

	s.Test("ds.Keys", func(t *testcase.T) {
		exp := t.Random.IntBetween(3, 7)
		assert.Equal(t, dsmap.Len(KVSKeys{Size: exp}), exp)
	})

	s.Test("ds.All", func(t *testcase.T) {
		exp := t.Random.IntBetween(3, 7)
		m := dsmap.Map[string, int](random.Map(exp, func() (string, int) {
			return t.Random.String(), t.Random.Int()
		}))
		assert.Equal(t, dsmap.Len(m), exp)
	})
}

type dummyMap[K comparable, V any] struct{}

var _ ds.ReadOnlyMap[string, int] = dummyMap[string, int]{}

func (m dummyMap[K, V]) Lookup(key K) (V, bool) { return *new(V), false }
func (m dummyMap[K, V]) Get(key K) V            { return *new(V) }
func (m dummyMap[K, V]) Set(key K, val V)       {}
func (m dummyMap[K, V]) Delete(key K)           {}
func (m dummyMap[K, V]) All() iter.Seq2[K, V]   { return func(yield func(K, V) bool) {} }

type KVSSizer struct {
	Size int
	dummyMap[string, int]
}

func (v KVSSizer) Len() int {
	return v.Size
}

type KVSKeys struct {
	Size int
	dummyMap[string, int]
}

func (v KVSKeys) Keys() iter.Seq[string] {
	var n int
	return func(yield func(string) bool) {
		for range v.Size {
			n++
			if !yield(strconv.Itoa(n)) {
				return
			}
		}
	}
}
