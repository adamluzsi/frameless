package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

type Deleter struct {
	T interface{}
	FixtureFactory
	Subject minimumRequirements
}

func (spec Deleter) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Describe(`DeleteByID`, spec.specDeleteByID)
	s.Describe(`DeleteAll`, spec.specDeleteAll)
}

func (spec Deleter) Benchmark(b *testing.B) {
	b.Run(`DeleteByID`, spec.benchmarkDeleteByID)
	b.Run(`DeleteAll`, spec.benchmarkDeleteAll)
}

func (spec Deleter) specDeleteByID(s *testcase.Spec) {

	subject := func(t *testcase.T) error {
		return spec.Subject.DeleteByID(
			t.I(`ctx`).(context.Context),
			spec.T,
			t.I(`id`),
		)
	}

	s.Let(`ctx`, func(t *testcase.T) interface{} {
		return spec.Context()
	})

	s.Before(func(t *testcase.T) {
		require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
	})

	s.Let(`entity`, func(t *testcase.T) interface{} {
		return spec.FixtureFactory.Create(spec.T)
	})

	s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.Create(spec.Context(), t.I(`entity`)))
		})

		s.Let(`id`, func(t *testcase.T) interface{} {
			id, ok := resources.LookupID(t.I(`entity`))
			require.True(t, ok, ErrIDRequired.Error())
			require.NotEmpty(t, id)
			return id
		})

		s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			e := newEntity(spec.T)
			found, err := spec.Subject.FindByID(spec.Context(), e, t.I(`id`))
			require.Nil(t, err)
			require.False(t, found)
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			s.Let(`oth-entity`, func(t *testcase.T) interface{} {
				return spec.FixtureFactory.Create(spec.T)
			})
			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Create(spec.Context(), t.I(`oth-entity`)))
			})

			s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				othID, ok := resources.LookupID(t.I(`oth-entity`))
				require.True(t, ok, ErrIDRequired.Error())

				e := newEntity(spec.T)
				found, err := spec.Subject.FindByID(spec.Context(), e, othID)
				require.Nil(t, err)
				require.True(t, found)
			})
		})

		s.And(`the entity was deleted then`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, subject(t))
			})

			s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})
	})

	s.When(`entity never saved before in the resource`, func(s *testcase.Spec) {
		s.Let(`id`, func(t *testcase.T) interface{} {
			id, _ := resources.LookupID(t.I(`entity`))
			return id
		})

		s.Before(func(t *testcase.T) {
			found, err := spec.Subject.FindByID(spec.Context(), newEntity(spec.T), t.I(`id`))
			require.Nil(t, err)
			require.False(t, found)
		})

		s.Then(`it will return with error, because you cannot delete something that does not exist`, func(t *testcase.T) {
			require.Error(t, subject(t))
		})
	})
}

func (spec Deleter) benchmarkDeleteByID(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)

	var total int
wrk:
	for {

		b.StopTimer()
		es := createEntities(spec.FixtureFactory, spec.T)
		ids := saveEntities(b, spec.Subject, spec.FixtureFactory, es...)
		b.StartTimer()

		for _, id := range ids {
			require.Nil(b, spec.Subject.DeleteByID(spec.FixtureFactory.Context(), spec.T, id))
			total++

			if total == b.N {
				break wrk
			}
		}

	}
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
		eID := spec.populateFor(t, spec.T)
		require.True(t, spec.isStored(t, eID, spec.T))
		require.Nil(t, subject(t))
		require.False(t, spec.isStored(t, eID, spec.T))
	})

	s.Then(`it should not affect other entities`, func(t *testcase.T) {
		t.Skip(`TODO/POC`)

		ff, ok := spec.FixtureFactory.(interface {
			OthEntityType(e interface{}) interface{}
		})
		if !ok {
			t.Skip(`OthEntityType not yet implemented`)
		}

		spec.populateFor(t, spec.T)
		othT := ff.OthEntityType(spec.T)
		oID := spec.populateFor(t, othT)

		require.True(t, spec.isStored(t, oID, othT))
		require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
		require.True(t, spec.isStored(t, oID, othT))
		require.Nil(t, spec.Subject.DeleteByID(spec.Context(), othT, oID))
	})
}

func (spec Deleter) benchmarkDeleteAll(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	// for some reason, doing setup with timer stop/start
	// makes this test unable to measure
	// the correct throughput, and hangs forever
	// so I just check empty db truncate then.
	// This anyway not a thing that is often used.
	for i := 0; i < b.N; i++ {
		require.Nil(b, spec.Subject.DeleteAll(spec.Context(), spec.T))
	}
	b.StopTimer()
}

func (spec Deleter) populateFor(t testing.TB, Type interface{}) interface{} {
	fixture := spec.FixtureFactory.Create(Type)
	require.Nil(t, spec.Subject.Create(spec.Context(), fixture))

	id, ok := resources.LookupID(fixture)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func (spec Deleter) isStored(t testing.TB, ID, Type interface{}) bool {
	entity := newEntity(Type)
	ok, err := spec.Subject.FindByID(spec.Context(), entity, ID)
	require.Nil(t, err)
	return ok
}
