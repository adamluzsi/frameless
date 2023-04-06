package crudcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"testing"

	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/frameless/ports/crud"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/testcase/let"

	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Finder[Entity, ID any] func(testing.TB) FinderSubject[Entity, ID]

type FinderSubject[Entity, ID any] struct {
	Resource interface {
		spechelper.CRD[Entity, ID]
		crud.AllFinder[Entity]
	}
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

func (c Finder[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Finder[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Finder[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		ByIDFinder[Entity, ID](func(tb testing.TB) ByIDFinderSubject[Entity, ID] {
			sub := c(tb)
			return ByIDFinderSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		AllFinder[Entity, ID](func(tb testing.TB) AllFinderSubject[Entity, ID] {
			sub := c(tb)
			return AllFinderSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
	)
}

type ByIDFinder[Entity, ID any] func(testing.TB) ByIDFinderSubject[Entity, ID]

type ByIDFinderSubject[Entity, ID any] struct {
	Resource    spechelper.CRD[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

func (c ByIDFinder[Entity, ID]) subject() testcase.Var[ByIDFinderSubject[Entity, ID]] {
	return testcase.Var[ByIDFinderSubject[Entity, ID]]{
		ID:     "ByIDFinderSubject[Entity, ID]",
		Init:   func(t *testcase.T) ByIDFinderSubject[Entity, ID] { return c(t) },
		Before: nil,
		OnLet:  nil,
	}
}

func (c ByIDFinder[Entity, ID]) Name() string {
	return "ByIDFinder"
}

func (c ByIDFinder[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s, QueryOne[Entity, ID](func(tb testing.TB) QueryOneSubject[Entity, ID] {
		sub := c(tb)
		return QueryOneSubject[Entity, ID]{
			Resource: sub.Resource,
			Name:     "FindByID",

			Query: func(ctx context.Context, ent Entity) (_ Entity, found bool, _ error) {
				id, ok := extid.Lookup[ID](ent)
				if !ok { // if no id found create a dummy ID
					// Since an id is always required to use FindByID,
					// we generate a dummy id if the received entity doesn't have one.
					// This helps to avoid error cases where ID is not actually set.
					// For those, we have further specifications later.
					id = c.createDummyID(tb.(*testcase.T), sub)
				}
				return sub.Resource.FindByID(ctx, id)
			},

			MakeContext: sub.MakeContext,
			MakeEntity:  sub.MakeEntity,

			Specify: func(tb testing.TB) {
				s := testcase.NewSpec(tb)
				defer s.Finish()

				var (
					ctx = let.With[context.Context](s, sub.MakeContext)
					id  = testcase.Let[ID](s, nil)
				)
				act := func(t *testcase.T) (Entity, bool, error) {
					return c.subject().Get(t).Resource.FindByID(ctx.Get(t), id.Get(t))
				}

				s.When("id points to an existing value", func(s *testcase.Spec) {
					ent := testcase.Let(s, func(t *testcase.T) Entity {
						var (
							e   = c.subject().Get(t).MakeEntity()
							ctx = c.subject().Get(t).MakeContext()
						)
						t.Must.NoError(c.subject().Get(t).Resource.Create(ctx, &e))
						t.Defer(c.subject().Get(t).Resource.DeleteByID, ctx, HasID[Entity, ID](t, e))
						return e
					})

					id.Let(s, func(t *testcase.T) ID {
						return HasID[Entity, ID](t, ent.Get(t))
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
							r   = c.subject().Get(t).Resource
							e   = c.subject().Get(t).MakeEntity()
							ctx = c.subject().Get(t).MakeContext()
						)
						t.Must.NoError(r.Create(ctx, &e))
						var id = HasID[Entity, ID](t, e)
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
		}
	}))
}

func (c ByIDFinder[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ByIDFinder[Entity, ID]) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)

	sub := c.subject().Bind(s)

	ent := testcase.Let(s, func(t *testcase.T) *Entity {
		ptr := new(Entity)
		Create[Entity, ID](t, sub.Get(t).Resource, c.subject().Get(t).MakeContext(), ptr)
		return ptr
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Entity, ID](t, pointer.Deref(ent.Get(t)))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		_, _, err := sub.Get(t).Resource.FindByID(c.subject().Get(t).MakeContext(), id.Get(t))
		t.Must.Nil(err)
	})
}

// AllFinder can return business entities from a given resource that implement it's test
// The "EntityTypeName" is an Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type AllFinder[Entity, ID any] func(testing.TB) AllFinderSubject[Entity, ID]

type AllFinderSubject[Entity, ID any] struct {
	Resource    allFinderSubjectResource[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity
}

type allFinderSubjectResource[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
	crud.AllFinder[Entity]
}

func (c AllFinder[Entity, ID]) Name() string {
	return "AllFinder"
}

func (c AllFinder[Entity, ID]) Spec(s *testcase.Spec) {
	QueryMany[Entity, ID](func(tb testing.TB) QueryManySubject[Entity, ID] {
		sub := c(tb)
		return QueryManySubject[Entity, ID]{
			Resource: sub.Resource,
			Query: func(ctx context.Context) iterators.Iterator[Entity] {
				return sub.Resource.FindAll(ctx)
			},
			MakeContext:        sub.MakeContext,
			MakeIncludedEntity: sub.MakeEntity,
			MakeExcludedEntity: nil, // intentionally empty
		}
	}).Spec(s)
}

func (c AllFinder[Entity, ID]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c AllFinder[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c ByIDFinder[Entity, ID]) createDummyID(t *testcase.T, sub ByIDFinderSubject[Entity, ID]) ID {
	ent := sub.MakeEntity()
	ctx := sub.MakeContext()
	Create[Entity, ID](t, sub.Resource, ctx, &ent)
	id := HasID[Entity, ID](t, ent)
	Delete[Entity, ID](t, sub.Resource, ctx, &ent)
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
