package crudcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/ports/crud"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
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
	ByIDFinderSubject[Entity, ID]
	AllFinderSubject[Entity, ID]
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
			t := tb.(*testcase.T)
			r := c.MakeSubject(t)

			var ids []ID
			for i := 0; i < 12; i++ {
				ent := c.MakeEntity(t)
				Create[Entity, ID](t, r, c.MakeContext(t), &ent)
				id, ok := extid.Lookup[ID](&ent)
				t.Must.True(ok, spechelper.ErrIDRequired.Error())
				ids = append(ids, id)
			}

			t.Log("when no value stored that the query request")
			ctx := c.MakeContext(t)
			_, ok, err := r.FindByID(c.MakeContext(t), c.createNonActiveID(t, ctx, r))
			t.Must.Nil(err)
			t.Must.False(ok)

			t.Log("values returned")
			for _, id := range ids {
				ent, ok, err := r.FindByID(c.MakeContext(t), id)
				t.Must.Nil(err)
				t.Must.True(ok)

				actualID, ok := extid.Lookup[ID](ent)
				t.Must.True(ok, "can't find ID in the returned value")
				t.Must.Equal(id, actualID)
			}
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

func (c ByIDFinder[Entity, ID]) createNonActiveID(tb testing.TB, ctx context.Context, r ByIDFinderSubject[Entity, ID]) ID {
	tb.Helper()
	ent := c.MakeEntity(tb)
	ptr := &ent
	Create[Entity, ID](tb, r, ctx, ptr)
	id, _ := extid.Lookup[ID](ptr)
	Delete[Entity, ID](tb, r, ctx, ptr)
	return id
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
