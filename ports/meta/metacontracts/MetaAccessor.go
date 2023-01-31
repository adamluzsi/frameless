package metacontracts

import (
	"context"
	"sync"
	"testing"

	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/frameless/ports/crud"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/meta"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type MetaAccessor[Entity, ID, V any] struct {
	MakeSubject func(testing.TB) MetaAccessorSubject[Entity, ID, V]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	MakeV       func(testing.TB) V
}

type MetaAccessorSubject[Entity any, ID any, V any] struct {
	meta.MetaAccessor
	Resource  spechelper.CRD[Entity, ID]
	Publisher interface {
		pubsub.CreatorPublisher[Entity]
		pubsub.UpdaterPublisher[Entity]
		pubsub.DeleterPublisher[ID]
	}
}

func (c MetaAccessor[Entity, ID, V]) metaAccessorSubject() testcase.Var[MetaAccessorSubject[Entity, ID, V]] {
	return testcase.Var[MetaAccessorSubject[Entity, ID, V]]{ID: `MetaAccessorSubject`}
}

func (c MetaAccessor[Entity, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessor[Entity, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessor[Entity, ID, V]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		MetaAccessorBasic[V]{
			MakeSubject: func(tb testing.TB) meta.MetaAccessor {
				return c.MakeSubject(tb).MetaAccessor
			},
			MakeV: c.MakeV,
		},
		MetaAccessorPublisher[Entity, ID, V]{
			MakeSubject: func(tb testing.TB) MetaAccessorSubject[Entity, ID, V] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
			MakeV:       c.MakeV,
		},
	)
}

// MetaAccessorBasic
// V is the value T type that can be set and looked up with frameless.MetaAccessor.
type MetaAccessorBasic[V any] struct {
	MakeSubject func(testing.TB) meta.MetaAccessor
	MakeV       func(testing.TB) V
}

func (c MetaAccessorBasic[V]) metaAccessorSubject() testcase.Var[meta.MetaAccessor] {
	return testcase.Var[meta.MetaAccessor]{ID: `MetaAccessorBasicSubject`}
}

func (c MetaAccessorBasic[V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorBasic[V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorBasic[V]) Spec(s *testcase.Spec) {
	c.metaAccessorSubject().Let(s, func(t *testcase.T) meta.MetaAccessor {
		return c.MakeSubject(t)
	})

	// SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	// LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
	s.Describe(`.SetMeta+.LookupMeta`, func(s *testcase.Spec) {
		var (
			ctx   = spechelper.ContextVar.Let(s, nil)
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
				t.Must.Equal(spechelper.Base(ptr), value.Get(t))
			})
		})
	})
}

type MetaAccessorPublisher[Entity any, ID any, V any] struct {
	MakeSubject func(testing.TB) MetaAccessorSubject[Entity, ID, V]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	MakeV       func(testing.TB) V
}

func (c MetaAccessorPublisher[Entity, ID, V]) subject() testcase.Var[MetaAccessorSubject[Entity, ID, V]] {
	return testcase.Var[MetaAccessorSubject[Entity, ID, V]]{
		ID: `subject`,
		Init: func(t *testcase.T) MetaAccessorSubject[Entity, ID, V] {
			return c.MakeSubject(t)
		},
	}
}

func (c MetaAccessorPublisher[Entity, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorPublisher[Entity, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorPublisher[Entity, ID, V]) Spec(s *testcase.Spec) {
	s.Test(".SetMeta -> .Create -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := c.MakeContext(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[Entity, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				_ = event.(pubsub.CreateEvent[Entity])
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
		v2 := c.MakeEntity(t)
		Create[Entity, ID](t, c.subject().Get(t).Resource, ctx, &v2)

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})

	s.Test(".SetMeta -> .DeleteByID -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := c.MakeContext(t)
		key := t.Random.String()
		expected := c.MakeV(t)
		ptr := spechelper.ToPtr(c.MakeEntity(t))

		Create[Entity, ID](t, c.subject().Get(t).Resource, ctx, ptr)
		id := HasID[Entity, ID](t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToDeleterEvents(ctx, doubles.StubSubscriber[Entity, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(pubsub.DeleteByIDEvent[ID]); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = spechelper.Base(v)
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
		ctx := c.MakeContext(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		ptr := spechelper.ToPtr(c.MakeEntity(t))
		Create[Entity, ID](t, c.subject().Get(t).Resource, ctx, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToDeleterEvents(ctx, doubles.StubSubscriber[Entity, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(pubsub.DeleteAllEvent); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = spechelper.Base(v)
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		ctx, err = c.subject().Get(t).SetMeta(ctx, key, expected)
		t.Must.Nil(err)

		allDeleter, ok := c.subject().Get(t).Resource.(crud.AllDeleter)
		if !ok {
			t.Skip()
		}
		t.Must.Nil(allDeleter.DeleteAll(ctx))

		Eventually.Assert(t, func(it assert.It) {
			mutex.RLock()
			defer mutex.RUnlock()
			it.Must.Equal(expected, actual)
		})
	})

	s.Test(".SetMeta -> .Update -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		res, ok := c.subject().Get(t).Resource.(crudcontracts.UpdaterSubject[Entity, ID])
		if !ok {
			t.Skipf(`frameless.Updater is not implemented by %T`, c.subject().Get(t).Resource)
		}

		ctx := c.MakeContext(t)
		key := t.Random.String()
		expected := c.MakeV(t)

		ptr := spechelper.ToPtr(c.MakeEntity(t))
		Create[Entity, ID](t, c.subject().Get(t).Resource, ctx, ptr)
		id := HasID[Entity, ID](t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := c.subject().Get(t).Publisher.SubscribeToUpdaterEvents(ctx, doubles.StubSubscriber[Entity, ID]{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(pubsub.UpdateEvent[Entity]); !ok {
					return nil
				}

				v := new(V)
				found, err := c.subject().Get(t).LookupMeta(ctx, key, v)
				t.Must.Nil(err)
				t.Must.True(found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = spechelper.Base(v)
				return nil
			},
		})
		t.Must.Nil(err)
		t.Defer(sub.Close)

		updPTR := spechelper.ToPtr(c.MakeEntity(t))
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
