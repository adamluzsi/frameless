package datastructcontract

import (
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func KVS[K comparable, V any](make func(tb testing.TB) datastruct.KVS[K, V], opts ...KVSOption[K, V]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("smoke", func(t *testcase.T) {
		var kvs = make(t)

		expected := map[K]V{}
		t.Random.Repeat(3, 7, func() {
			key := random.Unique(func() K {
				return c.makeK(t)
			}, mapkit.Keys(expected)...)
			expected[key] = c.makeV(t)
		})

		var expLen int
		for k, v := range expected {
			assert.Equal(t, kvs.Len(), expLen)
			assert.Empty(t, kvs.Get(k), "zero value was expected for getting a non stored value")
			_, ok := kvs.Lookup(k)
			assert.False(t, ok, assert.MessageF("%#v key was not expected to be found", k))

			kvs.Set(k, v)
			expLen++
			assert.Equal(t, kvs.Len(), expLen)
			got, ok := kvs.Lookup(k)
			assert.True(t, ok)
			assert.Equal(t, v, got)
			assert.Equal(t, v, kvs.Get(k))
		}

		kNoise, vNoise := c.makeK(t), c.makeV(t)
		kvs.Set(kNoise, vNoise)
		assert.Equal(t, expLen+1, kvs.Len())
		kvs.Delete(kNoise)
		assert.Equal(t, expLen, kvs.Len())
		_, ok := kvs.Lookup(kNoise)
		assert.False(t, ok)
		assert.Empty(t, kvs.Get(kNoise))

		assert.ContainsExactly(t, mapkit.Keys(expected), kvs.Keys())
		assert.ContainsExactly(t, expected, kvs.ToMap())
		assert.ContainsExactly(t, expected, iterkit.Collect2Map(kvs.Iter()))
	})

	s.Test("keys are unique in the store", func(t *testcase.T) {
		var kvs = make(t)
		k := c.makeK(t)
		t.Random.Repeat(3, 7, func() {
			kvs.Set(k, c.makeV(t))
		})
		assert.Equal(t, 1, kvs.Len())
		exp := c.makeV(t)
		kvs.Set(k, exp)
		assert.Equal(t, 1, kvs.Len())
		assert.Equal(t, exp, kvs.Get(k))
		kvs.Delete(k)
		assert.Equal(t, 0, kvs.Len())
	})

	s.Describe("#Iter", iterkitcontract.IterSeq2(func(tb testing.TB) iter.Seq2[K, V] {
		t := testcase.ToT(&tb)
		kvs := make(t)
		t.Random.Repeat(3, 7, func() {
			k := c.makeK(t)
			v := c.makeV(t)
			kvs.Set(k, v)
		})
		return kvs.Iter()
	}).Spec)

	kName := reflectkit.TypeOf[K]().String()
	vName := reflectkit.TypeOf[V]().String()
	return s.AsSuite(fmt.Sprintf("KVS[%s, %s]", kName, vName))
}

type KVSOption[K comparable, V any] interface {
	option.Option[KVSConfig[K, V]]
}

type KVSConfig[K comparable, V any] struct {
	MakeK func(testing.TB) K
	MakeV func(testing.TB) V
}

var _ KVSOption[string, int] = KVSConfig[string, int]{}

func (c KVSConfig[K, V]) Configure(o *KVSConfig[K, V]) {
	o.MakeK = zerokit.Coalesce(c.MakeK, o.MakeK)
	o.MakeV = zerokit.Coalesce(c.MakeV, o.MakeV)
}

func (c KVSConfig[K, V]) makeK(tb testing.TB) K {
	return zerokit.Coalesce(c.MakeK, spechelper.MakeValue[K])(tb)
}

func (c KVSConfig[K, V]) makeV(tb testing.TB) V {
	return zerokit.Coalesce(c.MakeV, spechelper.MakeValue[V])(tb)
}
