package crudcontract

import (
	"context"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/testcase"
)

func Deleter[ENT, ID any](subject crud.Deleter[ID], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe("DeleteByID", ByIDDeleter(subject, opts...).Spec)
	s.Describe("DeleteAll", AllDeleter(subject, opts...).Spec)
	return s.AsSuite("Deleter")
}

func ByIDDeleter[ENT, ID any](subject crud.ByIDDeleter[ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)

	var (
		Context = let.With[context.Context](s, c.MakeContext)
		id      = testcase.Var[ID]{ID: `id`}
	)
	act := func(t *testcase.T) error {
		return subject.DeleteByID(Context.Get(t), id.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), act)
	})

	ptr := testcase.Let(s, func(t *testcase.T) *ENT {
		return pointer.Of(c.MakeEntity(t))
	})

	s.When(`the request context is cancelled`, func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		id.Let(s, func(t *testcase.T) ID {
			var zero ID
			return zero
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	if creator, ok := subject.(crud.Creator[ENT]); ok {
		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			id.Let(s, func(t *testcase.T) ID {
				p := ptr.Get(t)
				c.Helper().Create(t, creator, c.MakeContext(t), p)
				return c.IDA.Get(*p)
			}).EagerLoading(s)

			if byIDFinder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
				s.Then(`the entity will no longer be find-able in the resource by the id`, func(t *testcase.T) {
					assert.NoError(t, act(t))

					c.Helper().IsAbsent(t, byIDFinder, c.MakeContext(t), id.Get(t))
				})

				s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
					othEntPtr := testcase.Let(s, func(t *testcase.T) ENT {
						var ent = c.MakeEntity(t)
						c.Helper().Create(t, creator, c.MakeContext(t), &ent)
						return ent
					}).EagerLoading(s)

					s.Then(`the other entity will be not affected by the operation`, func(t *testcase.T) {
						t.Must.NoError(act(t))

						c.Helper().IsPresent(t, byIDFinder, c.MakeContext(t), c.IDA.Get(othEntPtr.Get(t)))
					})
				})
			}

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				s.Then(`deletion will make FindAll not list the deleted entity`, func(t *testcase.T) {
					assert.NoError(t, act(t))

					t.Eventually(func(t *testcase.T) {
						for ent, err := range allFinder.FindAll(c.MakeContext(t)) {
							assert.NoError(t, err)
							assert.NotEqual(t, id.Get(t), c.IDA.Get(ent))
						}
					})
				})
			}

			s.And(`the entity was already deleted before`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.NoError(act(t))

					if r, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
						c.Helper().IsAbsent(t, r, c.MakeContext(t), id.Get(t))
					}
				})

				s.Then(`it will result in error for an already deleted entity`, func(t *testcase.T) {
					t.Must.ErrorIs(crud.ErrNotFound, act(t))
				})
			})

			s.And(`the request context is cancelled`, func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(c.MakeContext(t))
					cancel()
					return ctx
				})

				s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
					t.Must.ErrorIs(context.Canceled, act(t))
				})
			})
		})
	}

	return s.AsSuite("ByIDDeleter")
}

func AllDeleter[ENT, ID any](subject crud.AllDeleter, opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context { return c.MakeContext(t) })
	)
	act := func(t *testcase.T) error {
		return subject.DeleteAll(ctx.Get(t))
	}

	s.Benchmark("", func(t *testcase.T) {
		assert.NoError(t, subject.DeleteAll(c.MakeContext(t)))
	})

	s.When(`the request context is cancelled`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	if creator, ok := subject.(crud.Creator[ENT]); ok {
		s.When("an entity is created in the resource", func(s *testcase.Spec) {
			entity := let.Var(s, func(t *testcase.T) *ENT {
				ent := c.MakeEntity(t)
				c.Helper().Create(t, creator, c.MakeContext(t), &ent)
				return &ent
			}).EagerLoading(s)

			if byIDFinder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
				s.Then(`deletion will make FindByID yield not found for the entity`, func(t *testcase.T) {
					assert.NoError(t, act(t))

					id, ok := c.IDA.Lookup(*entity.Get(t))
					assert.True(t, ok, assert.MessageF("%T doesn't have an external resource ID", *entity.Get(t)))

					c.Helper().IsAbsent(t, byIDFinder, c.MakeContext(t), id)
				})
			}

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				s.Then(`deletion will make FindAll yield an empty result`, func(t *testcase.T) {
					assert.NoError(t, act(t))

					t.Eventually(func(t *testcase.T) {
						n, err := iterkit.CountE(allFinder.FindAll(c.MakeContext(t)))
						assert.NoError(t, err)
						assert.Empty(t, n)
					})
				})
			}
		})
	}

	if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
		s.Then(`deletion will make FindAll yield an empty result`, func(t *testcase.T) {
			assert.NoError(t, act(t))

			t.Eventually(func(t *testcase.T) {
				n, err := iterkit.CountE(allFinder.FindAll(c.MakeContext(t)))
				assert.NoError(t, err)
				assert.Empty(t, n)
			})
		})
	}

	return s.AsSuite("AllDeleter")
}
