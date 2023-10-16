package metacontracts

import (
	"context"
	"github.com/adamluzsi/testcase/let"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/ports/meta"
	"github.com/adamluzsi/testcase"
)

// MetaAccessor
// V is the value T type that can be set and looked up with frameless.MetaAccessor.
type MetaAccessor[V any] func(testing.TB) MetaAccessorSubject[V]

type MetaAccessorSubject[V any] struct {
	MetaAccessor meta.MetaAccessor
	MakeContext  func() context.Context
	MakeV        func() V
}

func (c MetaAccessor[V]) metaAccessorSubject() testcase.Var[meta.MetaAccessor] {
	return testcase.Var[meta.MetaAccessor]{ID: `MetaAccessorBasicSubject`}
}

func (c MetaAccessor[V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessor[V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessor[V]) Spec(s *testcase.Spec) {
	subject := let.With[MetaAccessorSubject[V]](s, (func(testing.TB) MetaAccessorSubject[V])(c))

	c.metaAccessorSubject().Let(s, func(t *testcase.T) meta.MetaAccessor {
		return subject.Get(t).MetaAccessor
	})

	// SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	// LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
	s.Describe(`.SetMeta+.LookupMeta`, func(s *testcase.Spec) {
		var (
			ctx   = testcase.Let(s, func(t *testcase.T) context.Context { return subject.Get(t).MakeContext() })
			key   = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
			value = testcase.Let(s, func(t *testcase.T) V { return subject.Get(t).MakeV() })
		)
		setMeta := func(t *testcase.T) (context.Context, error) {
			return c.metaAccessorSubject().Get(t).SetMeta(ctx.Get(t), key.Get(t), value.Get(t))
		}
		lookupMeta := func(t *testcase.T, ptr interface{} /*[V]*/) (bool, error) {
			return c.metaAccessorSubject().Get(t).LookupMeta(ctx.Get(t), key.Get(t), ptr)
		}

		s.Test(`on an empty context the lookup will yield no result without an issue`, func(t *testcase.T) {
			found, err := lookupMeta(t, new(V))
			t.Must.Nil(err)
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
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(pointer.Deref(ptr), value.Get(t))
			})
		})
	})
}
