package crudcontract

import (
	"context"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

// Updater will request an update for a wrapped entity object in the Resource
func Updater[ENT, ID any](subject crud.Updater[ENT], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	store, sOK := storer[ENT, ID](c, subject)
	d, dOK := subject.(crud.ByIDDeleter[ID])
	f, fOK := subject.(crud.ByIDFinder[ENT, ID])

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), subject)
	})

	var (
		requestContext = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		entityWithChanges = testcase.Var[*ENT]{ID: `entity-with-changes`}
	)
	act := func(t *testcase.T) error {
		return subject.Update(
			requestContext.Get(t),
			entityWithChanges.Get(t),
		)
	}

	updaterBenchmark[ENT, ID](s, subject, c)

	if sOK {
		s.When(`an entity already stored`, func(s *testcase.Spec) {
			ptr := testcase.Let(s, func(t *testcase.T) *ENT {
				ent := pointer.Of(c.MakeEntity(t))
				store(t, ent)
				return ent
			}).EagerLoading(s)

			s.And(`the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
				entityWithChanges.Let(s, func(t *testcase.T) *ENT {
					v := *ptr.Get(t)
					c.ModifyEntity(t, &v)
					return &v
				})

				if fOK {
					s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
						assert.Must(t).NoError(act(t))

						c.Helper().HasEntity(t, f, c.MakeContext(t), entityWithChanges.Get(t))
					})
				}

				s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
					requestContext.Let(s, func(t *testcase.T) context.Context {
						ctx, cancel := context.WithCancel(c.MakeContext(t))
						cancel()
						return ctx
					})

					s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
						assert.Must(t).ErrorIs(context.Canceled, act(t))
					})
				})
			})
		})
	}

	if sOK && dOK {
		s.When(`the received entity has ext.ID that is unknown in the repository`, func(s *testcase.Spec) {
			entityWithChanges.Let(s, func(t *testcase.T) *ENT {
				newEntity := pointer.Of(c.MakeEntity(t))
				store(t, newEntity)
				c.Helper().Delete(t, d, c.MakeContext(t), newEntity)
				return newEntity
			})

			s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
				t.Must.ErrorIs(crud.ErrNotFound, act(t))
			})
		})
	}

	return s.AsSuite("Updater")
}

func updaterBenchmark[ENT, ID any](s *testcase.Spec, subject crud.Updater[ENT], c Config[ENT, ID]) {
	ent := testcase.Let(s, func(t *testcase.T) *ENT {
		ptr := pointer.Of(c.MakeEntity(t))
		shouldStore(t, c, subject, ptr)
		return ptr
	})

	s.Benchmark("", func(t *testcase.T) {
		t.Must.NoError(subject.Update(c.MakeContext(t), ent.Get(t)))
	})
}
