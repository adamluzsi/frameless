package crudcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/frameless/ports/crud"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/testcase/let"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Finder[Entity, ID any] struct {
	MakeSubject func(testing.TB) FinderSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type FinderSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.AllFinder[Entity]
}

func (c Finder[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Finder[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Finder[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		ByIDFinder[Entity, ID]{
			MakeSubject: func(tb testing.TB) ByIDFinderSubject[Entity, ID] {
				return c.MakeSubject(tb).(ByIDFinderSubject[Entity, ID])
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		AllFinder[Entity, ID]{
			MakeSubject: func(tb testing.TB) AllFinderSubject[Entity, ID] {
				return c.MakeSubject(tb).(AllFinderSubject[Entity, ID])
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
	)
}

type ByIDFinder[Entity, ID any] struct {
	MakeSubject func(testing.TB) ByIDFinderSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type ByIDFinderSubject[Entity, ID any] spechelper.CRD[Entity, ID]

func (c ByIDFinder[Entity, ID]) Name() string {
	return "Finder.FindByID"
}

func (c ByIDFinder[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s, QueryOne[Entity, ID]{
		MakeSubject: func(tb testing.TB) QueryOneSubject[Entity, ID] {
			res := c.MakeSubject(tb)
			return QueryOneSubject[Entity, ID]{
				Resource: res,
				Query: func(tb testing.TB, ctx context.Context, ent Entity) (_ Entity, found bool, _ error) {
					id, ok := extid.Lookup[ID](ent)
					if !ok { // if no id found create a dummy ID
						// Since an id is always required to use FindByID,
						// we generate a dummy id if the received entity doesn't have one.
						// This helps to avoid error cases where ID is not actually set.
						// For those, we have further specifications later.
						id = c.createDummyID(tb.(*testcase.T), res)
					}
					return res.FindByID(ctx, id)
				},
			}
		},
		MakeContext: c.MakeContext,
		MakeEntity:  c.MakeEntity,
		QueryName:   "FindByID",

		Specify: func(tb testing.TB) {
			s := testcase.NewSpec(tb)
			defer s.Finish()

			var (
				resource = let.With[ByIDFinderSubject[Entity, ID]](s, c.MakeSubject)
				ctx      = let.With[context.Context](s, c.MakeContext)
				id       = testcase.Let[ID](s, nil)
			)
			act := func(t *testcase.T) (Entity, bool, error) {
				return resource.Get(t).FindByID(ctx.Get(t), id.Get(t))
			}

			s.When("id points to an existing value", func(s *testcase.Spec) {
				ent := testcase.Let(s, func(t *testcase.T) Entity {
					var (
						e   = c.MakeEntity(t)
						ctx = c.MakeContext(t)
					)
					t.Must.NoError(resource.Get(t).Create(ctx, &e))
					t.Defer(resource.Get(t).DeleteByID, ctx, HasID[Entity, ID](t, &e))
					return e
				})

				id.Let(s, func(t *testcase.T) ID {
					return HasID[Entity, ID](t, pointer.Of(ent.Get(t)))
				})

				s.Then("it will find and return the entity", func(t *testcase.T) {
					Eventually.Assert(t, func(it assert.It) {
						got, found, err := act(t)
						it.Must.NoError(err)
						it.Must.True(found)
						it.Must.Equal(ent.Get(t), got)
					})
				})
			})

			s.When("id points to an already deleted value", func(s *testcase.Spec) {
				id.Let(s, func(t *testcase.T) ID {
					var (
						r   = resource.Get(t)
						e   = c.MakeEntity(t)
						ctx = c.MakeContext(t)
					)
					t.Must.NoError(r.Create(ctx, &e))
					var id = HasID[Entity, ID](t, &e)
					Eventually.Assert(t, func(it assert.It) {
						_, found, err := r.FindByID(ctx, id)
						it.Must.NoError(err)
						it.Must.True(found)
					})
					t.Must.NoError(r.DeleteByID(ctx, id))
					return id
				}).EagerLoading(s)

				s.Then("it reports that the entity is not found", func(t *testcase.T) {
					Eventually.Assert(t, func(it assert.It) {
						_, ok, err := act(t)
						it.Must.Nil(err)
						it.Must.False(ok)
					})
				})
			})
		},
	})
}

func (c ByIDFinder[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ByIDFinder[Entity, ID]) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)
	r := c.MakeSubject(b)

	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		ptr := new(Entity)
		Create[Entity, ID](t, r, c.MakeContext(t), ptr)
		return ptr
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Entity, ID](t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		_, _, err := r.FindByID(c.MakeContext(t), id.Get(t))
		t.Must.Nil(err)
	})
}

// AllFinder can return business entities from a given resource that implement it's test
// The "EntityTypeName" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type AllFinder[Entity, ID any] struct {
	MakeSubject func(testing.TB) AllFinderSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type AllFinderSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.AllFinder[Entity]
}

func (c AllFinder[Entity, ID]) Name() string {
	return "Finder.FindAll"
}

func (c AllFinder[Entity, ID]) Spec(s *testcase.Spec) {
	QueryMany[Entity, ID]{
		MakeSubject: func(tb testing.TB) QueryManySubject[Entity, ID] {
			resource := c.MakeSubject(tb)
			return QueryManySubject[Entity, ID]{
				Resource: resource,
				MakeQuery: func(tb testing.TB, ctx context.Context) iterators.Iterator[Entity] {
					return resource.FindAll(ctx)
				},
			}
		},
		MakeContext:        c.MakeContext,
		MakeIncludedEntity: c.MakeEntity,
		MakeExcludedEntity: nil, // intentionally empty
	}.Spec(s)
}

func (c AllFinder[Entity, ID]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c AllFinder[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c ByIDFinder[Entity, ID]) createDummyID(t *testcase.T, r ByIDFinderSubject[Entity, ID]) ID {
	ent := c.MakeEntity(t)
	ctx := c.MakeContext(t)
	Create[Entity, ID](t, r, ctx, &ent)
	id := HasID[Entity, ID](t, &ent)
	Delete[Entity, ID](t, r, ctx, &ent)
	return id
}

func (c AllFinder[Entity, ID]) findAllN(t *testcase.T, subject func(t *testcase.T) iterators.Iterator[Entity], n int) []Entity {
	var entities []Entity
	Eventually.Assert(t, func(tb assert.It) {
		var err error
		all := subject(t)
		entities, err = iterators.Collect(all)
		assert.Must(tb).Nil(err)
		assert.Must(tb).Equal(n, len(entities))
	})
	return entities
}
