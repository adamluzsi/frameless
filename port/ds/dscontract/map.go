package dscontract

import (
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dsmap"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Map[K comparable, V any](mk func(tb testing.TB) ds.Map[K, V], opts ...MapOption[K, V]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("smoke", func(t *testcase.T) {
		var kvs = mk(t)

		expected := map[K]V{}
		t.Random.Repeat(3, 7, func() {
			key := makeUniqueValue(t, c.MakeKey)
			expected[key] = makeValue(t, c.MakeValue)
		})

		var expLen int
		for k, v := range expected {

			assert.Equal(t, expLen, dsmap.Len(kvs))
			assert.Empty(t, kvs.Get(k), "zero value was expected for getting a non stored value")
			_, ok := kvs.Lookup(k)
			assert.False(t, ok, assert.MessageF("%#v key was not expected to be found", k))

			kvs.Set(k, v)
			expLen++
			assert.Equal(t, expLen, dsmap.Len(kvs))
			got, ok := kvs.Lookup(k)
			assert.True(t, ok)
			assert.Equal(t, v, got)
			assert.Equal(t, v, kvs.Get(k))
		}

		kNoise, vNoise := makeUniqueValue(t, c.MakeKey), makeValue(t, c.MakeValue)
		kvs.Set(kNoise, vNoise)
		assert.Equal(t, expLen+1, dsmap.Len(kvs))
		kvs.Delete(kNoise)
		assert.Equal(t, expLen, dsmap.Len(kvs))
		_, ok := kvs.Lookup(kNoise)
		assert.False(t, ok)
		assert.Empty(t, kvs.Get(kNoise))

		assert.ContainsExactly(t, mapkit.Keys(expected), iterkit.Collect(kvs.Keys()))
		assert.ContainsExactly(t, expected, iterkit.Collect2Map(kvs.All()))
	})

	s.Test("keys are unique in the store", func(t *testcase.T) {
		var kvs = mk(t)
		k := makeValue(t, c.MakeKey)
		t.Random.Repeat(3, 7, func() {
			kvs.Set(k, makeValue(t, c.MakeValue))
		})
		assert.Equal(t, 1, dsmap.Len(kvs))
		exp := makeValue(t, c.MakeValue)
		kvs.Set(k, exp)
		assert.Equal(t, 1, dsmap.Len(kvs))
		assert.Equal(t, exp, kvs.Get(k))
		kvs.Delete(k)
		assert.Equal(t, 0, dsmap.Len(kvs))
	})

	s.Describe("#Values", iterkitcontract.IterSeq(func(tb testing.TB) iter.Seq[V] {
		t := testcase.ToT(&tb)
		kvs := mk(t)
		vs, ok := kvs.(ds.Values[V])
		if !ok {
			tb.Skipf("ds.ValuesE[%s] is not supported by %T", reflectkit.TypeOf[V]().String(), kvs)
		}
		t.Random.Repeat(3, 7, func() {
			k := makeValue(t, c.MakeKey)
			v := makeValue(t, c.MakeValue)
			kvs.Set(k, v)
		})
		return vs.Values()
	}).Spec)

	s.Describe("#All", iterkitcontract.IterSeq2(func(tb testing.TB) iter.Seq2[K, V] {
		t := testcase.ToT(&tb)
		kvs := mk(t)
		t.Random.Repeat(3, 7, func() {
			k := makeValue(t, c.MakeKey)
			v := makeValue(t, c.MakeValue)
			kvs.Set(k, v)
		})
		return kvs.All()
	}).Spec)

	kName := reflectkit.TypeOf[K]().String()
	vName := reflectkit.TypeOf[V]().String()
	return s.AsSuite(fmt.Sprintf("Map[%s, %s]", kName, vName))
}

type MapOption[K comparable, V any] option.Option[MapConfig[K, V]]

type MapConfig[K comparable, V any] struct {
	MakeKey   func(testing.TB) K
	MakeValue func(testing.TB) V
}

var _ MapOption[string, int] = MapConfig[string, int]{}

func (c MapConfig[K, V]) Configure(o *MapConfig[K, V]) {
	o.MakeKey = zerokit.Coalesce(c.MakeKey, o.MakeKey)
	o.MakeValue = zerokit.Coalesce(c.MakeValue, o.MakeValue)
}
