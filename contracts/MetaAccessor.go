package contracts

import (
	"context"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type MetaAccessor[Ent, ID, V any] struct {
	Subject func(testing.TB) MetaAccessorSubject[Ent, ID, V]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
	MakeV   func(testing.TB) V
}

type MetaAccessorSubject[Ent any, ID any, V any] struct {
	frameless.MetaAccessor
	Resource  CRD[Ent, ID]
	Publisher interface {
		frameless.CreatorPublisher[Ent]
		frameless.UpdaterPublisher[Ent]
		frameless.DeleterPublisher[ID]
	}
}

func (c MetaAccessor[Ent, ID, V]) metaAccessorSubject() testcase.Var[MetaAccessorSubject[Ent, ID, V]] {
	return testcase.Var[MetaAccessorSubject[Ent, ID, V]]{ID: `MetaAccessorSubject`}
}

func (c MetaAccessor[Ent, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessor[Ent, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessor[Ent, ID, V]) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		MetaAccessorBasic[V]{
			Subject: func(tb testing.TB) frameless.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: c.MakeV,
		},
		MetaAccessorPublisher[Ent, ID, V]{
			Subject: func(tb testing.TB) MetaAccessorSubject[Ent, ID, V] {
				return c.Subject(tb)
			},
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
			MakeV:   c.MakeV,
		},
	)
}

// MetaAccessorBasic
// V is the value T type that can be set and looked up with frameless.MetaAccessor.
type MetaAccessorBasic[V any] struct {
	Subject func(testing.TB) frameless.MetaAccessor
	MakeV   func(testing.TB) V
}

func (c MetaAccessorBasic[V]) metaAccessorSubject() testcase.Var[frameless.MetaAccessor] {
	return testcase.Var[frameless.MetaAccessor]{ID: `MetaAccessorBasicSubject`}
}

func (c MetaAccessorBasic[V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorBasic[V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorBasic[V]) Spec(s *testcase.Spec) {
	c.metaAccessorSubject().Let(s, func(t *testcase.T) frameless.MetaAccessor {
		return c.Subject(t)
	})

	// SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	// LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
	s.Describe(`.SetMeta+.LookupMeta`, func(s *testcase.Spec) {
		var (
			ctx   = ctxVar.Let(s, nil)
			key   = testcase.Let(s, func(t *testcase.T) string { return t.Random.String() })
			value = testcase.Let(s, func(t *testcase.T) V { return c.MakeV(t) })
		)
		subjectSetMeta := func(t *testcase.T) (context.Context, error) {
			return c.metaAccessorSubject().Get(t).SetMeta(ctx.Get(t), key.Get(t), value.Get(t))
		}
		subjectLookupMeta := func(t *testcase.T, ptr interface{} /*[V]*/) (bool, error) {
			return c.metaAccessorSubject().Get(t).LookupMeta(ctx.Get(t), key.Get(t), ptr)
		}

		s.Test(`on an empty context the lookup will yield no result without an issue`, func(t *testcase.T) {
			found, err := subjectLookupMeta(t, new(V))
			t.Must.Nil(err)
			t.Must.False(found)
		})

		s.When(`value is set in a context`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				newContext, err := subjectSetMeta(t)
				t.Must.Nil(err)
				ctx.Set(t, newContext)
			})

			s.Then(`value can be found with lookup`, func(t *testcase.T) {
				ptr := new(V)
				found, err := subjectLookupMeta(t, ptr)
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(base(ptr), value.Get(t))
			})
		})
	})
}

type MetaAccessorPublisher[Ent any, ID any, V any] struct {
	Subject func(testing.TB) MetaAccessorSubject[Ent, ID, V]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
	MakeV   func(testing.TB) V
}

func (c MetaAccessorPublisher[Ent, ID, V]) subject() testcase.Var[MetaAccessorSubject[Ent, ID, V]] {
	return testcase.Var[MetaAccessorSubject[Ent, ID, V]]{
		ID: `subject`,
		Init: func(t *testcase.T) MetaAccessorSubject[Ent, ID, V] {
			return c.Subject(t)
		},
	}
}

func (c MetaAccessorPublisher[Ent, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorPublisher[Ent, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorPublisher[Ent, ID, V]) Spec(s *testcase.Spec) {
	s.Test(".SetMeta -> .Create -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := c.Context(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[Ent, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				_ = event.(frameless.CreateEvent[Ent])
				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = *v
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		ctx, err = c.subject().Get(t).SetMeta(ctx, key, expected)
		t.Must.Nil(err)
		v2 := c.MakeEnt(t)
		Create[Ent, ID](t, c.subject().Get(t).Resource, ctx, &v2)

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})

	s.Test(".SetMeta -> .DeleteByID -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := c.Context(t)
		key := t.Random.String()
		expected := c.MakeV(t)
		ptr := toPtr(c.MakeEnt(t))

		Create[Ent, ID](t, c.subject().Get(t).Resource, ctx, ptr)
		id := HasID[Ent, ID](t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToDeleterEvents(ctx, doubles.StubSubscriber[Ent, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.DeleteByIDEvent[ID]); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		ctx, err = c.subject().Get(t).SetMeta(ctx, key, expected)
		t.Must.Nil(err)
		t.Must.Nil(c.subject().Get(t).Resource.DeleteByID(ctx, id))

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})

	s.Test(".SetMeta -> .DeleteAll -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := c.Context(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		ptr := toPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.subject().Get(t).Resource, ctx, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToDeleterEvents(ctx, doubles.StubSubscriber[Ent, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.DeleteAllEvent); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		ctx, err = c.subject().Get(t).SetMeta(ctx, key, expected)
		t.Must.Nil(err)
		t.Must.Nil(c.subject().Get(t).Resource.DeleteAll(ctx))

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})

	s.Test(".SetMeta -> .Update -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		res, ok := c.subject().Get(t).Resource.(UpdaterSubject[Ent, ID])
		if !ok {
			t.Skipf(`frameless.Updater is not implemented by %T`, c.subject().Get(t).Resource)
		}

		ctx := c.Context(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		ptr := toPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.subject().Get(t).Resource, ctx, ptr)
		id := HasID[Ent, ID](t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToUpdaterEvents(ctx, doubles.StubSubscriber[Ent, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.UpdateEvent[Ent]); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		updPTR := toPtr(c.MakeEnt(t))
		t.Must.Nil(extid.Set(updPTR, id))
		ctx, err = c.subject().Get(t).SetMeta(ctx, key, expected)
		t.Must.Nil(err)
		t.Must.Nil(res.Update(ctx, updPTR))

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})
}
