package metacontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/meta"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
)

// MetaAccessor
// V is the value T type that can be set and looked up with frameless.MetaAccessor.
func MetaAccessor[V any](subject meta.MetaAccessor, opts ...Option[V]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[V]](opts)

	// SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	// LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
	s.Describe(`.SetMeta+.LookupMeta`, func(s *testcase.Spec) {
		var (
			ctx   = testcase.Let(s, func(t *testcase.T) context.Context { return c.MakeContext(t) })
			key   = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
			value = testcase.Let(s, func(t *testcase.T) V { return c.MakeV(t) })
		)
		setMeta := func(t *testcase.T) (context.Context, error) {
			return subject.SetMeta(ctx.Get(t), key.Get(t), value.Get(t))
		}
		lookupMeta := func(t *testcase.T, ptr interface{} /*[V]*/) (bool, error) {
			return subject.LookupMeta(ctx.Get(t), key.Get(t), ptr)
		}

		s.Test(`on an empty context the lookup will yield no result without an issue`, func(t *testcase.T) {
			found, err := lookupMeta(t, new(V))
			t.Must.NoError(err)
			t.Must.False(found)
		})

		s.When(`value is set in a context`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				newContext, err := setMeta(t)
				t.Must.NoError(err)
				ctx.Set(t, newContext)
			})

			s.Then(`value can be found with lookup`, func(t *testcase.T) {
				ptr := new(V)
				found, err := lookupMeta(t, ptr)
				t.Must.NoError(err)
				t.Must.True(found)
				t.Must.Equal(pointer.Deref(ptr), value.Get(t))
			})
		})
	})

	return s.AsSuite("MetaAccessor")
}

type Option[V any] interface {
	option.Option[Config[V]]
}

type Config[V any] struct {
	MakeV       func(tb testing.TB) V
	MakeContext func(tb testing.TB) context.Context
}

func (c *Config[V]) Init() {
	c.MakeV = spechelper.MakeValue[V]
	c.MakeContext = func(tb testing.TB) context.Context {
		return context.Background()
	}
}

func (c Config[V]) Configure(t *Config[V]) {
	*t = reflectkit.MergeStruct(*t, c)
}
