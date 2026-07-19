package dscontract

import (
	"context"
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
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func MapE[K comparable, V any](mk func(tb testing.TB) ds.MapE[K, V], opts ...MapEOption[K, V]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("smoke", func(t *testcase.T) {
		var kvs = mk(t)

		ctx := makeContext(t, c.MakeContext)

		expected := map[K]V{}
		t.Random.Repeat(3, 7, func() {
			key := makeUniqueValue(t, c.MakeKey)
			expected[key] = makeValue(t, c.MakeValue)
		})

		var expLen int
		for k, v := range expected {

			assert.Equal(t, expLen, lenMapE[K, V](t, ctx, kvs))

			got, err := kvs.Get(ctx, k)
			assert.NoError(t, err)
			assert.Empty(t, got, "zero value was expected for getting a non stored value")

			_, ok, err := kvs.Lookup(ctx, k)
			assert.NoError(t, err)
			assert.False(t, ok, assert.MessageF("%#v key was not expected to be found", k))

			assert.NoError(t, kvs.Set(ctx, k, v))
			expLen++
			assert.Equal(t, expLen, lenMapE[K, V](t, ctx, kvs))

			gotVal, ok, err := kvs.Lookup(ctx, k)
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, v, gotVal)

			gotGet, err := kvs.Get(ctx, k)
			assert.NoError(t, err)
			assert.Equal(t, v, gotGet)
		}

		kNoise, vNoise := makeUniqueValue(t, c.MakeKey), makeValue(t, c.MakeValue)
		assert.NoError(t, kvs.Set(ctx, kNoise, vNoise))
		assert.Equal(t, expLen+1, lenMapE[K, V](t, ctx, kvs))
		assert.NoError(t, kvs.Delete(ctx, kNoise))
		assert.Equal(t, expLen, lenMapE[K, V](t, ctx, kvs))

		_, ok, err := kvs.Lookup(ctx, kNoise)
		assert.NoError(t, err)
		assert.False(t, ok)

		gotNoise, err := kvs.Get(ctx, kNoise)
		assert.NoError(t, err)
		assert.Empty(t, gotNoise)

		gotKeys, err := iterkit.CollectE(kvs.Keys(ctx))
		assert.NoError(t, err)
		assert.ContainsExactly(t, mapkit.Keys(expected), gotKeys)
	})

	s.Test("keys are unique in the store", func(t *testcase.T) {
		var kvs = mk(t)

		ctx := makeContext(t, c.MakeContext)

		k := makeValue(t, c.MakeKey)
		t.Random.Repeat(3, 7, func() {
			assert.NoError(t, kvs.Set(ctx, k, makeValue(t, c.MakeValue)))
		})
		assert.Equal(t, 1, lenMapE[K, V](t, ctx, kvs))

		exp := makeValue(t, c.MakeValue)
		assert.NoError(t, kvs.Set(ctx, k, exp))
		assert.Equal(t, 1, lenMapE[K, V](t, ctx, kvs))

		got, err := kvs.Get(ctx, k)
		assert.NoError(t, err)
		assert.Equal(t, exp, got)

		assert.NoError(t, kvs.Delete(ctx, k))
		assert.Equal(t, 0, lenMapE[K, V](t, ctx, kvs))
	})

	s.Describe("#KeysE", iterkitcontract.IterSeq2(func(tb testing.TB) iter.Seq2[K, error] {
		t := testcase.ToT(&tb)
		kvs := mk(t)
		ctx := makeContext(t, c.MakeContext)
		t.Random.Repeat(3, 7, func() {
			k := makeValue(t, c.MakeKey)
			v := makeValue(t, c.MakeValue)
			assert.NoError(t, kvs.Set(ctx, k, v))
		})
		return kvs.Keys(ctx)
	}).Spec)

	s.Describe("#ValuesE", iterkitcontract.IterSeq2(func(tb testing.TB) iter.Seq2[V, error] {
		t := testcase.ToT(&tb)
		kvs := mk(t)
		vs, ok := kvs.(ds.ValuesE[V])
		if !ok {
			tb.Skipf("ds.ValuesE[%s] is not supported by %T", reflectkit.TypeOf[V]().String(), kvs)
		}
		ctx := makeContext(t, c.MakeContext)
		t.Random.Repeat(3, 7, func() {
			k := makeValue(t, c.MakeKey)
			v := makeValue(t, c.MakeValue)
			assert.NoError(t, kvs.Set(ctx, k, v))
		})
		return vs.Values(ctx)
	}).Spec)

	s.Describe("#All", iterkitcontract.IterSeq2(func(tb testing.TB) iter.Seq2[ds.KeyValuePair[K, V], error] {
		t := testcase.ToT(&tb)
		kvs := mk(t)
		t.Random.Repeat(3, 7, func() {
			ctx := makeContext(t, c.MakeContext)
			k := makeValue(t, c.MakeKey)
			v := makeValue(t, c.MakeValue)
			kvs.Set(ctx, k, v)
		})

		all, ok := kvs.(interface {
			All(ctx context.Context) iter.Seq2[ds.KV[K, V], error]
		})
		if !ok {
			tb.Skipf("AllE is not supported by %T", kvs)
		}
		ctx := makeContext(t, c.MakeContext)
		src := all.All(ctx)
		return func(yield func(ds.KeyValuePair[K, V], error) bool) {
			for kv, err := range src {
				if !yield(kv, err) {
					return
				}
			}
		}
	}).Spec)

	kName := reflectkit.TypeOf[K]().String()
	vName := reflectkit.TypeOf[V]().String()
	return s.AsSuite(fmt.Sprintf("MapE[%s, %s]", kName, vName))
}

type MapEOption[K comparable, V any] option.Option[MapEConfig[K, V]]

type MapEConfig[K comparable, V any] struct {
	MakeContext func(testing.TB) context.Context
	MakeKey     func(testing.TB) K
	MakeValue   func(testing.TB) V
}

var _ MapEOption[string, int] = MapEConfig[string, int]{}

func (c MapEConfig[K, V]) Configure(o *MapEConfig[K, V]) {
	o.MakeContext = zerokit.Coalesce(c.MakeContext, o.MakeContext)
	o.MakeKey = zerokit.Coalesce(c.MakeKey, o.MakeKey)
	o.MakeValue = zerokit.Coalesce(c.MakeValue, o.MakeValue)
}
