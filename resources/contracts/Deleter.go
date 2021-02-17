package contracts

import (
	"context"
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

type Deleter struct {
	Subject minimumRequirements
	T       interface{}
	FixtureFactory
}

func (spec Deleter) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	defer s.Finish()
	debug(s, spec)
	s.Describe(`DeleteByID`, spec.specDeleteByID)
	s.Describe(`DeleteAll`, spec.specDeleteAll)
}

func (spec Deleter) Benchmark(b *testing.B) {
	b.Run(`DeleteByID`, spec.benchmarkDeleteByID)
	b.Run(`DeleteAll`, spec.benchmarkDeleteAll)
}

func (spec Deleter) specDeleteByID(s *testcase.Spec) {
	var (
		ctx     = ctxLetWithFixtureFactory(s, spec)
		id      = testcase.Var{Name: `id`}
		subject = func(t *testcase.T) error {
			return spec.Subject.DeleteByID(ctx.Get(t).(context.Context), spec.T, id.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		DeleteAllEntity(t, spec.Subject, spec.Context(), spec.T)
	})

	entity := s.Let(`entity`, func(t *testcase.T) interface{} {
		return spec.FixtureFactory.Create(spec.T)
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) interface{} {
			ent := entity.Get(t)
			CreateEntity(t, spec.Subject, spec.Context(), ent)
			id, ok := resources.LookupID(ent)
			require.True(t, ok, ErrIDRequired.Error())
			return id
		}).EagerLoading(s)

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			othEntity := s.Let(`oth-entity`, func(t *testcase.T) interface{} {
				ent := spec.FixtureFactory.Create(spec.T)
				CreateEntity(t, spec.Subject, spec.Context(), ent)
				return ent
			}).EagerLoading(s)

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				othID, _ := resources.LookupID(othEntity.Get(t))
				IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), othID)
			})
		})

		s.And(`the entity was deleted then`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
				IsAbsent(t, spec.Subject, ctx.Get(t).(context.Context), newEntityFunc(spec.T), id.Get(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})
	})

	//s.When(`entity never saved before in the resource`, func(s *testcase.spec) {
	//	s.Let(`id`, func(t *testcase.T) interface{} {
	//		id, _ := resources.LookupID(entity.Get(t))
	//		return id
	//	})
	//
	//	s.Before(func(t *testcase.T) {
	//		found, err := spec.Subject.FindByID(spec.Context(), newEntity(spec.T), id.Get(t))
	//		require.Nil(t, err)
	//		require.False(t, found)
	//	})
	//
	//	s.Then(`it will return with error, because you cannot delete something that does not exist`, func(t *testcase.T) {
	//		require.Error(t, subject(t))
	//	})
	//})
}

func (spec Deleter) benchmarkDeleteByID(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)

	s := testcase.NewSpec(b)

	ent := s.Let(`ent`, func(t *testcase.T) interface{} {
		ptr := newEntity(spec.T)
		CreateEntity(t, spec.Subject, spec.Context(), ptr)
		return ptr
	}).EagerLoading(s)

	id := s.Let(`id`, func(t *testcase.T) interface{} {
		return HasID(t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		require.Nil(b, spec.Subject.DeleteByID(spec.Context(), spec.T, id.Get(t)))
	})
}

func (spec Deleter) specDeleteAll(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		return spec.Subject.DeleteAll(
			t.I(`ctx`).(context.Context),
			spec.T,
		)
	}

	s.Let(`ctx`, func(t *testcase.T) interface{} { return spec.Context() })

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		s.Let(`ctx`, func(t *testcase.T) interface{} {
			ctx, cancel := context.WithCancel(spec.Context())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
			require.Equal(t, context.Canceled, subject(t))
		})
	})

	s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
		ent := spec.FixtureFactory.Create(spec.T)
		CreateEntity(t, spec.Subject, spec.Context(), ent)
		eID := HasID(t, ent)
		IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), eID)
		require.Nil(t, subject(t))
		IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), eID)
	})
}

func (spec Deleter) benchmarkDeleteAll(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	// for some reason, doing setup with timer stop/start
	// makes this test unable to measure
	// the correct throughput, and hangs forever
	// so I just check empty delete all.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.Nil(b, spec.Subject.DeleteAll(spec.Context(), spec.T))
	}
	b.StopTimer()
}
