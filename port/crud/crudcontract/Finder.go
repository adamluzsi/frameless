package crudcontract

import (
	"context"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/testcase/let"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

type subjectFinder[ENT, ID any] interface {
	crud.ByIDFinder[ENT, ID]
	crud.AllFinder[ENT]
}

func Finder[ENT, ID any](subject subjectFinder[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe("ByIDFinder", ByIDFinder[ENT, ID](subject, opts...).Spec)
	s.Describe("AllFinder", AllFinder[ENT, ID](subject, opts...).Spec)
	return s.AsSuite("Finder")
}

func ByIDFinder[ENT, ID any](subject crud.ByIDFinder[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	var mkEnt = func(t *testcase.T) ENT {
		mk := func() ENT {
			return zerokit.Coalesce(c.ExampleEntity, c.MakeEntity)(t)
		}
		return makeEntity(t, t.SkipNow, c, subject, mk, "Config.ExampleEntity / Config.MakeEntity")
	}

	s.Describe("FindByID", func(s *testcase.Spec) {
		var (
			ctx = let.With[context.Context](s, c.MakeContext)
			id  = testcase.Let[ID](s, nil)
		)
		act := func(t *testcase.T) (ENT, bool, error) {
			return subject.FindByID(ctx.Get(t), id.Get(t))
		}

		s.When("id points to an existing value", func(s *testcase.Spec) {
			ent := testcase.Let(s, func(t *testcase.T) ENT {
				return mkEnt(t)
			})

			id.Let(s, func(t *testcase.T) ID {
				return c.Helper().HasID(t, pointer.Of(ent.Get(t)))
			})

			s.Then("it will find and return the entity", func(t *testcase.T) {
				crudtest.Eventually.Assert(t, func(it testing.TB) {
					got, found, err := act(t)
					assert.NoError(it, err)
					assert.True(it, found)
					assert.Equal(it, ent.Get(t), got)
				})
			})
		})

		if deleter, ok := subject.(crud.ByIDDeleter[ID]); ok {
			s.When("id points to an already deleted value", func(s *testcase.Spec) {
				id.Let(s, func(t *testcase.T) ID {

					var (
						ctx = c.MakeContext(t)
						ent = mkEnt(t)
						id  = c.Helper().HasID(t, &ent)
					)
					crudtest.Eventually.Assert(t, func(it testing.TB) {
						_, found, err := subject.FindByID(ctx, id)
						assert.NoError(it, err)
						assert.True(it, found)
					})
					t.Must.NoError(deleter.DeleteByID(ctx, id))
					crudtest.Eventually.Assert(t, func(it testing.TB) {
						_, found, err := subject.FindByID(ctx, id)
						assert.NoError(it, err)
						assert.False(it, found)
					})
					return id
				}).EagerLoading(s)

				s.Then("it reports that the entity is not found", func(t *testcase.T) {
					crudtest.Eventually.Assert(t, func(it testing.TB) {
						_, ok, err := act(t)
						assert.NoError(it, err)
						assert.False(it, ok)
					})
				})
			})
		}

		QueryOne[ENT, ID](subject, "#FindByID", func(tb testing.TB) QueryOneSubject[ENT] {
			ent := ensureExistingEntity(tb, c, subject, nil)

			return QueryOneSubject[ENT]{
				Query: func(ctx context.Context) (_ ENT, found bool, _ error) {
					return subject.FindByID(ctx, c.IDA.Get(ent))
				},
				ExpectedEntity: ent,
				ExcludedEntity: func() ENT {
					return ensureExistingEntity(tb, c, subject, func() ENT {
						return c.MakeEntity(tb)
					})
				},
			}
		}, opts...)
	})

	return s.AsSuite("ByIDFinder")
}

// AllFinder can return business entities from a given resource that implement it's test
// The "EntityTypeName" is an Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
func AllFinder[ENT, ID any](subject crud.AllFinder[ENT], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig[Config[ENT, ID]](opts)
	return QueryMany[ENT, ID](subject, "#FindAll", func(t testing.TB) QueryManySubject[ENT] {
		return QueryManySubject[ENT]{
			Query: subject.FindAll,
			IncludedEntity: func() ENT {
				return zerokit.Coalesce(c.ExampleEntity, c.MakeEntity)(t)
			},
			// ExcludedEntity is left empty on purpose
			// since it's not relevant for creating an entity that AllFinder shouldn't return.
			ExcludedEntity: nil,
		}
	}, opts...)
}

func ByIDsFinder[ENT, ID any](subject crud.ByIDsFinder[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		ids = testcase.Var[[]ID]{ID: `entities ids`}
	)
	var act = func(t *testcase.T) iter.Seq2[ENT, error] {
		return subject.FindByIDs(ctx.Get(t), ids.Get(t)...)
	}

	var mkEnt = func(t *testcase.T) ENT {
		return makeEntity[ENT, ID](t, t.SkipNow, c, subject, func() ENT {
			return zerokit.Coalesce(c.ExampleEntity, c.MakeEntity)(t)
		}, "Config.ExampleEntity / Config.MakeEntity")
	}

	var (
		newEntityInit = func(t *testcase.T) *ENT {
			ent := mkEnt(t)
			return &ent
		}
		ent1 = testcase.Let(s, newEntityInit)
		ent2 = testcase.Let(s, newEntityInit)
	)

	s.When(`id list is empty`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{}
		})

		s.Then(`result is an empty list`, func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})

	s.When(`id list contains ids stored in the repository`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{c.IDA.Get(*ent1.Get(t)), c.IDA.Get(*ent2.Get(t))}
		})

		s.Then(`it will return all entities`, func(t *testcase.T) {
			expected := append([]ENT{}, *ent1.Get(t), *ent2.Get(t))
			actual, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			t.Must.ContainExactly(expected, actual)
		})
	})

	if deleter, ok := subject.(crud.ByIDDeleter[ID]); ok {
		s.When(`id list contains at least one id that doesn't have stored entity`, func(s *testcase.Spec) {
			ids.Let(s, func(t *testcase.T) []ID {
				return []ID{c.IDA.Get(*ent1.Get(t)), c.IDA.Get(*ent2.Get(t))}
			})

			s.Before(func(t *testcase.T) {
				c.Helper().Delete(t, deleter, ctx.Get(t), ent1.Get(t))
			})

			s.Then(`it will yield error early on`, func(t *testcase.T) {
				_, _, err := iterkit.FirstE(act(t))
				assert.ErrorIs(t, err, crud.ErrNotFound)
			})
		})
	}

	return s.AsSuite("ByIDsFinder")
}
