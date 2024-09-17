package crudcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	crudtest "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

type subjectFinder[Entity, ID any] interface {
	crud.ByIDFinder[Entity, ID]
	crud.AllFinder[Entity]
}

func Finder[Entity, ID any](subject subjectFinder[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	s.Describe("ByIDFinder", ByIDFinder[Entity, ID](subject, opts...).Spec)
	s.Describe("AllFinder", AllFinder[Entity, ID](subject, opts...).Spec)
	return s.AsSuite("Finder")
}

func ByIDFinder[Entity, ID any](subject crud.ByIDFinder[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var mkEnt = func(t *testcase.T) Entity {
		return makeEntity(t, t.SkipNow, c, subject, zerokit.Coalesce(c.ExampleEntity, c.MakeEntity), "Config.ExampleEntity / Config.MakeEntity")
	}

	s.Describe("FindByID", func(s *testcase.Spec) {
		var (
			ctx = let.With[context.Context](s, c.MakeContext)
			id  = testcase.Let[ID](s, nil)
		)
		act := func(t *testcase.T) (Entity, bool, error) {
			return subject.FindByID(ctx.Get(t), id.Get(t))
		}

		s.When("id points to an existing value", func(s *testcase.Spec) {
			ent := testcase.Let(s, func(t *testcase.T) Entity {
				return mkEnt(t)
			})

			id.Let(s, func(t *testcase.T) ID {
				return crudtest.HasID[Entity, ID](t, pointer.Of(ent.Get(t)))
			})

			s.Then("it will find and return the entity", func(t *testcase.T) {
				crudtest.Eventually.Assert(t, func(it assert.It) {
					got, found, err := act(t)
					it.Must.NoError(err)
					it.Must.True(found)
					it.Must.Equal(ent.Get(t), got)
				})
			})
		})

		if deleter, ok := subject.(crud.ByIDDeleter[ID]); ok {
			s.When("id points to an already deleted value", func(s *testcase.Spec) {
				id.Let(s, func(t *testcase.T) ID {

					var (
						ctx = c.MakeContext()
						ent = mkEnt(t)
						id  = crudtest.HasID[Entity, ID](t, &ent)
					)
					crudtest.Eventually.Assert(t, func(it assert.It) {
						_, found, err := subject.FindByID(ctx, id)
						it.Must.NoError(err)
						it.Must.True(found)
					})
					t.Must.NoError(deleter.DeleteByID(ctx, id))
					crudtest.Eventually.Assert(t, func(it assert.It) {
						_, found, err := subject.FindByID(ctx, id)
						it.Must.NoError(err)
						it.Must.False(found)
					})
					return id
				}).EagerLoading(s)

				s.Then("it reports that the entity is not found", func(t *testcase.T) {
					crudtest.Eventually.Assert(t, func(it assert.It) {
						_, ok, err := act(t)
						it.Must.Nil(err)
						it.Must.False(ok)
					})
				})
			})
		}

		QueryOne[Entity, ID](subject, func(tb testing.TB, ctx context.Context, ent Entity) (_ Entity, found bool, _ error) {
			id, ok := lookupID[ID](c, ent)
			if !ok { // if no id found create a dummy ID
				// Since an id is always required to use FindByID,
				// we generate a dummy id if the received entity doesn't have one.
				// This helps to avoid error cases where ID is not actually set.
				// For those, we have further specifications later.
				if subject, ok := subject.(crd[Entity, ID]); ok {
					id = createDummyID[Entity, ID](ctx.Value(TestingTBContextKey{}).(*testcase.T), subject, c)
				} else {
					id = tb.(*testcase.T).Random.Make(reflectkit.TypeOf[ID]()).(ID)
				}
			}
			return subject.FindByID(ctx, id)
		}, opts...)
	})

	return s.AsSuite("ByIDFinder")
}

// AllFinder can return business entities from a given resource that implement it's test
// The "EntityTypeName" is an Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
func AllFinder[Entity, ID any](subject crud.AllFinder[Entity], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	return QueryMany[Entity, ID](subject,
		func(tb testing.TB, ctx context.Context) iterators.Iterator[Entity] {
			return subject.FindAll(ctx)
		},
		zerokit.Coalesce(c.ExampleEntity, c.MakeEntity),
		nil, // intentionally empty as it is not applicable to create an entity that is not returned by AllFinder
		c,
	)
}

func ByIDsFinder[ENT, ID any](subject crud.ByIDsFinder[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	c := option.Use[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext()
		})
		ids = testcase.Var[[]ID]{ID: `entities ids`}
	)
	var act = func(t *testcase.T) iterators.Iterator[ENT] {
		return subject.FindByIDs(ctx.Get(t), ids.Get(t)...)
	}

	var mkEnt = func(t *testcase.T) ENT {
		return makeEntity[ENT, ID](t, t.SkipNow, c, subject, zerokit.Coalesce(c.ExampleEntity, c.MakeEntity), "Config.ExampleEntity / Config.MakeEntity")
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
			count, err := iterators.Count(act(t))
			t.Must.Nil(err)
			t.Must.Equal(0, count)
		})
	})

	s.When(`id list contains ids stored in the repository`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{getID[ENT, ID](t, c, *ent1.Get(t)), getID[ENT, ID](t, c, *ent2.Get(t))}
		})

		s.Then(`it will return all entities`, func(t *testcase.T) {
			expected := append([]ENT{}, *ent1.Get(t), *ent2.Get(t))
			actual, err := iterators.Collect(act(t))
			t.Must.Nil(err)
			t.Must.ContainExactly(expected, actual)
		})
	})

	if deleter, ok := subject.(crudtest.CRD[ENT, ID]); ok {
		s.When(`id list contains at least one id that doesn't have stored entity`, func(s *testcase.Spec) {
			ids.Let(s, func(t *testcase.T) []ID {
				return []ID{getID[ENT, ID](t, c, *ent1.Get(t)), getID[ENT, ID](t, c, *ent2.Get(t))}
			})

			s.Before(func(t *testcase.T) {
				crudtest.Delete[ENT, ID](t, deleter, ctx.Get(t), ent1.Get(t))
			})

			s.Then(`it will eventually yield error`, func(t *testcase.T) {
				_, err := iterators.Collect(act(t))
				t.Must.NotNil(err)
			})
		})
	}

	return s.AsSuite("ByIDsFinder")
}
