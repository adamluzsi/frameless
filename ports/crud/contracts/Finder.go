package crudcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/crud"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"sync"
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
	testcase.RunSuite(s, FindOne[Entity, ID]{
		MakeSubject: func(tb testing.TB) FindOneSubject[Entity, ID] {
			return c.MakeSubject(tb)
		},
		MakeContext: c.MakeContext,
		MakeEntity:  c.MakeEntity,
		MethodName:  "FindByID",
		ToQuery: func(tb testing.TB, resource FindOneSubject[Entity, ID], ent Entity) QueryOne[Entity] {
			id, ok := extid.Lookup[ID](ent)
			if !ok { // if no id found create a dummy ID
				// Since an id is always required to use FindByID,
				// we generate a dummy id if the received entity doesn't have one.
				// This helps to avoid error cases where ID is not actually set.
				// For those, we have further specifications later.
				id = c.createDummyID(tb.(*testcase.T), resource)
			}

			return func(tb testing.TB, ctx context.Context) (ent Entity, found bool, err error) {
				return resource.FindByID(ctx, id)
			}
		},

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
	resource := testcase.Let(s, func(t *testcase.T) AllFinderSubject[Entity, ID] {
		return c.MakeSubject(t)
	})

	beforeAll := &sync.Once{}
	s.Before(func(t *testcase.T) {
		beforeAll.Do(func() { spechelper.TryCleanup(t, c.MakeContext(t), resource.Get(t)) })
	})

	s.Describe(`FindAll`, func(s *testcase.Spec) {
		var (
			ctx = testcase.Let(s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})
			subject = func(t *testcase.T) iterators.Iterator[Entity] {
				return resource.Get(t).FindAll(ctx.Get(t))
			}
		)

		s.Before(func(t *testcase.T) {
			spechelper.TryCleanup(t, c.MakeContext(t), resource.Get(t))
		})

		entity := testcase.Let(s, func(t *testcase.T) *Entity {
			ent := c.MakeEntity(t)
			return &ent
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				Create[Entity, ID](t, resource.Get(t), c.MakeContext(t), entity.Get(t))
			})

			s.Then(`the entity will returns the all the entity in volume`, func(t *testcase.T) {
				Eventually.Assert(t, func(tb assert.It) {
					count, err := iterators.Count(subject(t))
					assert.Must(tb).Nil(err)
					assert.Must(tb).Equal(1, count)
				})
			})

			s.Then(`the returned iterator includes the stored entity`, func(t *testcase.T) {
				Eventually.Assert(t, func(tb assert.It) {
					entities := c.findAllN(t, subject, 1)
					spechelper.Contains[Entity](tb, entities, *entity.Get(t))
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				othEntity := testcase.Let(s, func(t *testcase.T) *Entity {
					ent := c.MakeEntity(t)
					Create[Entity, ID](t, resource.Get(t), c.MakeContext(t), &ent)
					return &ent
				}).EagerLoading(s)

				s.Then(`all entity will be fetched`, func(t *testcase.T) {
					Eventually.Assert(t, func(tb assert.It) {
						entities := c.findAllN(t, subject, 2)
						spechelper.Contains[Entity](tb, entities, *entity.Get(t))
						spechelper.Contains[Entity](tb, entities, *othEntity.Get(t))
					})
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				spechelper.TryCleanup(t, c.MakeContext(t), resource.Get(t))
			})

			s.Then(`the iterator will have no result`, func(t *testcase.T) {
				count, err := iterators.Count(subject(t))
				t.Must.Nil(err)
				t.Must.Equal(0, count)
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				iter := subject(t)
				_ = iter.Next()
				err := iter.Err()
				t.Must.NotNil(err)
				t.Must.Equal(context.Canceled, err)
			})
		})
	})
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

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// QueryOne is the generic representation of a query that meant to find one result.
// It is really similar to resources.Finder#FindByID,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOne is generated through ToQuery factory function in FindOne resource contract specification.
type QueryOne[Entity any] func(tb testing.TB, ctx context.Context) (ent Entity, found bool, err error)

type FindOne[Entity, ID any] struct {
	MakeSubject func(testing.TB) FindOneSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	// MethodName is the name of the test subject QueryOne method of this contract specification.
	MethodName string
	// ToQuery takes an entity ptr and returns with a closure that has the knowledge about how to query on the MakeSubject resource to find the entity.
	//
	// By convention, any preparation action that affect the resource must take place prior to returning the closure.
	// The QueryOne closure should only have the Method call with the already mapped values.
	// ToQuery will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	ToQuery func(tb testing.TB, resource FindOneSubject[Entity, ID], ent Entity) QueryOne[Entity]
	// Specify allow further specification describing for a given FindOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

type FindOneSubject[Entity, ID any] spechelper.CRD[Entity, ID]

func (c FindOne[Entity, ID]) Name() string {
	return fmt.Sprintf(".%s", c.MethodName)
}

func (c FindOne[Entity, ID]) Spec(s *testcase.Spec) {
	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		entity = testcase.Let(s, func(t *testcase.T) *Entity {
			ent := c.MakeEntity(t)
			return &ent
		})
		resource = testcase.Let(s, func(t *testcase.T) FindOneSubject[Entity, ID] {
			return c.MakeSubject(t)
		})
		query = testcase.Let(s, func(t *testcase.T) QueryOne[Entity] {
			t.Log(entity.Get(t))
			return c.ToQuery(t, resource.Get(t), *entity.Get(t))
		})
		subject = func(t *testcase.T) (Entity, bool, error) {
			return query.Get(t)(t, ctx.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), resource.Get(t))
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Create[Entity, ID](t, resource.Get(t), c.MakeContext(t), entity.Get(t))
			HasID[Entity, ID](t, entity.Get(t))
		})

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			ent, found, err := subject(t)
			t.Must.Nil(err)
			t.Must.True(found)
			t.Must.Equal(*entity.Get(t), ent)
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				_, found, err := subject(t)
				t.Must.ErrorIs(context.Canceled, err)
				t.Must.False(found)
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ent := c.MakeEntity(t)
				Create[Entity, ID](t, resource.Get(t), c.MakeContext(t), &ent)
			})

			s.Then(`still the correct entity is returned`, func(t *testcase.T) {
				ent, found, err := subject(t)
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(*entity.Get(t), ent)
			})
		})
	})

	s.When(`no entity is saved in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			spechelper.TryCleanup(t, c.MakeContext(t), resource.Get(t))
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := subject(t)
			t.Must.Nil(err)
			t.Must.False(found)
		})
	})

	if c.Specify != nil {
		s.Test(`Specify`, func(t *testcase.T) {
			c.Specify(t)
		})
	}
}

func (c FindOne[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c FindOne[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
