package contracts

import (
	"context"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/stubs"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type MetaAccessor struct {
	T, V           T
	Subject        func(testing.TB) MetaAccessorSubject
	FixtureFactory func(testing.TB) FixtureFactory
}

var accessor = testcase.Var{Name: `frameless.MetaAccessor`}

func accessorGet(t *testcase.T) frameless.MetaAccessor {
	return accessor.Get(t).(frameless.MetaAccessor)
}

type MetaAccessorSubject struct {
	frameless.MetaAccessor
	CRD
	frameless.Publisher
}

var metaAccessorSubject = testcase.Var{Name: `MetaAccessorSubject`}

func metaAccessorSubjectGet(t *testcase.T) MetaAccessorSubject {
	return metaAccessorSubject.Get(t).(MetaAccessorSubject)
}

func (c MetaAccessor) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessor) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessor) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		MetaAccessorBasic{V: c.V,
			Subject: func(tb testing.TB) frameless.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			FixtureFactory: c.FixtureFactory,
		},
		MetaAccessorPublisher{T: c.T, V: c.V,
			Subject: func(tb testing.TB) MetaAccessorSubject {
				return c.Subject(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
	)
}

type MetaAccessorBasic struct {
	// V is the value T type that can be set and looked up with frameless.MetaAccessor.
	V              T
	Subject        func(testing.TB) frameless.MetaAccessor
	FixtureFactory func(testing.TB) FixtureFactory
}

func (c MetaAccessorBasic) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorBasic) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorBasic) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)
	accessor.Let(s, func(t *testcase.T) interface{} {
		return c.Subject(t)
	})

	// SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	// LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
	s.Describe(`.SetMeta+.LookupMeta`, func(s *testcase.Spec) {
		var (
			ctx    = ctx.Let(s, nil)
			key    = s.Let(`key`, func(t *testcase.T) interface{} { return t.Random.String() })
			keyGet = func(t *testcase.T) string { return key.Get(t).(string) }
			value  = s.Let(`value`, func(t *testcase.T) interface{} { return factoryGet(t).Create(c.V) })
		)
		subjectSetMeta := func(t *testcase.T) (context.Context, error) {
			return accessorGet(t).SetMeta(ctxGet(t), keyGet(t), value.Get(t))
		}
		subjectLookupMeta := func(t *testcase.T, ptr interface{} /*[V]*/) (bool, error) {
			return accessorGet(t).LookupMeta(ctxGet(t), keyGet(t), ptr)
		}

		s.Test(`on an empty context the lookup will yield no result without an issue`, func(t *testcase.T) {
			found, err := subjectLookupMeta(t, newT(c.V))
			require.NoError(t, err)
			require.False(t, found)
		})

		s.When(`value is set in a context`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				newContext, err := subjectSetMeta(t)
				require.NoError(t, err)
				ctx.Set(t, newContext)
			})

			s.Then(`value can be found with lookup`, func(t *testcase.T) {
				ptr := newT(c.V)
				found, err := subjectLookupMeta(t, ptr)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, base(ptr), value.Get(t))
			})
		})
	})
}

type MetaAccessorPublisher struct {
	T, V           T
	Subject        func(testing.TB) MetaAccessorSubject
	FixtureFactory func(testing.TB) FixtureFactory
}

func (c MetaAccessorPublisher) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c MetaAccessorPublisher) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c MetaAccessorPublisher) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)
	metaAccessorSubject.Let(s, func(t *testcase.T) interface{} {
		return c.Subject(t)
	})
	accessor.Let(s, func(t *testcase.T) interface{} {
		return metaAccessorSubjectGet(t).MetaAccessor
	})

	s.Test(".SetMeta -> .Create -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := factoryGet(t).Context()
		key := t.Random.String()
		expected := base(factoryGet(t).Create(c.V))

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := metaAccessorSubjectGet(t).Publisher.Subscribe(ctx, stubs.Subscriber{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.EventCreate); !ok {
					return nil
				}
				v := newT(c.V)
				found, err := metaAccessorSubjectGet(t).LookupMeta(ctx, key, v)
				require.NoError(t, err)
				require.True(t, found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		require.NoError(t, err)
		t.Defer(sub.Close)

		ctx, err = accessorGet(t).SetMeta(ctx, key, expected)
		require.NoError(t, err)
		CreateEntity(t, metaAccessorSubjectGet(t).CRD, ctx, CreatePTR(factoryGet(t), c.T))

		AsyncTester.Assert(t, func(t testing.TB) {
			mutex.RLock()
			defer mutex.RUnlock()
			require.Equal(t, expected, actual)
		})
	})

	s.Test(".SetMeta -> .DeleteByID -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := factoryGet(t).Context()
		key := t.Random.String()
		expected := base(factoryGet(t).Create(c.V))

		ptr := CreatePTR(factoryGet(t), c.T)
		CreateEntity(t, metaAccessorSubjectGet(t).CRD, ctx, ptr)
		id := HasID(t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := metaAccessorSubjectGet(t).Publisher.Subscribe(ctx, stubs.Subscriber{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.EventDeleteByID); !ok {
					return nil
				}

				v := newT(c.V)
				found, err := metaAccessorSubjectGet(t).LookupMeta(ctx, key, v)
				require.NoError(t, err)
				require.True(t, found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		require.NoError(t, err)
		t.Defer(sub.Close)

		ctx, err = accessorGet(t).SetMeta(ctx, key, expected)
		require.NoError(t, err)
		require.Nil(t, metaAccessorSubjectGet(t).CRD.DeleteByID(ctx, id))

		AsyncTester.Assert(t, func(t testing.TB) {
			mutex.RLock()
			defer mutex.RUnlock()
			require.Equal(t, expected, actual)
		})
	})

	s.Test(".SetMeta -> .DeleteAll -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		ctx := factoryGet(t).Context()
		key := t.Random.String()
		expected := base(factoryGet(t).Create(c.V))

		ptr := CreatePTR(factoryGet(t), c.T)
		CreateEntity(t, metaAccessorSubjectGet(t).CRD, ctx, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := metaAccessorSubjectGet(t).Publisher.Subscribe(ctx, stubs.Subscriber{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.EventDeleteAll); !ok {
					return nil
				}

				v := newT(c.V)
				found, err := metaAccessorSubjectGet(t).LookupMeta(ctx, key, v)
				require.NoError(t, err)
				require.True(t, found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		require.NoError(t, err)
		t.Defer(sub.Close)

		ctx, err = accessorGet(t).SetMeta(ctx, key, expected)
		require.NoError(t, err)
		require.Nil(t, metaAccessorSubjectGet(t).CRD.DeleteAll(ctx))

		AsyncTester.Assert(t, func(t testing.TB) {
			mutex.RLock()
			defer mutex.RUnlock()
			require.Equal(t, expected, actual)
		})
	})

	s.Test(".SetMeta -> .Update -> .Subscribe -> .LookupMeta", func(t *testcase.T) {
		crud, ok := metaAccessorSubjectGet(t).CRD.(UpdaterSubject)
		if !ok {
			t.Skipf(`frameless.Updater is not implemented by %T`, metaAccessorSubjectGet(t).CRD)
		}

		ctx := factoryGet(t).Context()
		key := t.Random.String()
		expected := base(factoryGet(t).Create(c.V))

		ptr := CreatePTR(factoryGet(t), c.T)
		CreateEntity(t, metaAccessorSubjectGet(t).CRD, ctx, ptr)
		id := HasID(t, ptr)

		var (
			actual interface{}
			mutex  sync.RWMutex
		)
		sub, err := metaAccessorSubjectGet(t).Publisher.Subscribe(ctx, stubs.Subscriber{
			HandleFunc: func(ctx context.Context, event interface{}) error {
				if _, ok := event.(frameless.EventUpdate); !ok {
					return nil
				}

				v := newT(c.V)
				found, err := metaAccessorSubjectGet(t).LookupMeta(ctx, key, v)
				require.NoError(t, err)
				require.True(t, found)
				mutex.Lock()
				defer mutex.Unlock()
				actual = base(v)
				return nil
			},
		})
		require.NoError(t, err)
		t.Defer(sub.Close)

		updPTR := CreatePTR(factoryGet(t), c.T)
		require.NoError(t, extid.Set(updPTR, id))
		ctx, err = accessorGet(t).SetMeta(ctx, key, expected)
		require.NoError(t, err)
		require.Nil(t, crud.Update(ctx, updPTR))

		AsyncTester.Assert(t, func(t testing.TB) {
			mutex.RLock()
			defer mutex.RUnlock()
			require.Equal(t, expected, actual)
		})
	})
}
