package dscontract

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func keys[K any]() testcase.Var[[]K] {
	var name = reflectkit.TypeOf[K]().String()
	return testcase.Var[[]K]{
		ID: testcase.VarID(fmt.Sprintf("kvs generated keys %s", name)),
		Init: func(t *testcase.T) []K {
			return []K{}
		},
	}
}

func makeUniqueValue[V any](t *testcase.T, mk func(testing.TB) V) V {
	key := random.Unique(func() V { return makeValue(t, mk) }, keys[V]().Get(t)...)
	testcase.Append(t, keys[V](), key)
	return key
}

func makeValue[V any](tb testing.TB, mk func(testing.TB) V) V {
	return zerokit.Coalesce(mk, spechelper.MakeValue[V])(tb)
}

func lenMapE[K, V any](tb testing.TB, ctx context.Context, v any) int {
	tb.Helper()
	if v, ok := v.(ds.LenE); ok {
		n, err := v.Len(ctx)
		assert.NoError(tb, err)
		return n
	}
	if v, ok := v.(ds.KeysE[K]); ok {
		count, err := iterkit.CountE(v.Keys(ctx))
		assert.NoError(tb, err)
		return count
	}
	type AllE = func(ctx context.Context) iter.Seq2[ds.KeyValuePair[K, V], error]
	if m, ok := reflectkit.AssertMethod[AllE](reflect.ValueOf(v), "AllE"); ok {
		count, err := iterkit.CountE(m(ctx))
		assert.NoError(tb, err)
		return count
	}
	var msg = fmt.Sprintf("unable to verify len of %T", v)
	tb.Skip(msg)
	panic(msg)
}

func makeContext(tb testing.TB, mk func(testing.TB) context.Context) context.Context {
	if mk != nil {
		return mk(tb)
	}
	return tb.Context()
}
