package contracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/frameless/iterators"
)

type Finder[Ent, ID any] struct {
	Subject func(testing.TB) FinderSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type FinderSubject[Ent, ID any] CRD[Ent, ID]

func (c Finder[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Finder[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Finder[Ent, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		findByID[Ent, ID]{
			Subject: c.Subject,
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		findAll[Ent, ID]{
			Subject: c.Subject,
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
	)
}

type findByID[Ent, ID any] struct {
	Subject func(testing.TB) FinderSubject[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c findByID[Ent, ID]) String() string {
	return "Finder.FindByID"
}

func (c findByID[Ent, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s, FindOne[Ent, ID]{
		Subject:    c.Subject,
		MakeCtx:    c.Context,
		MakeEnt:    c.MakeEnt,
		MethodName: "FindByID",
		ToQuery: func(tb testing.TB, resource FinderSubject[Ent, ID], ent Ent) QueryOne[Ent] {
			id, ok := extid.Lookup[ID](ent)
			if !ok { // if no id found create a dummy ID
				// Since an id is always required to use FindByID,
				// we generate a dummy id if the received entity doesn't have one.
				// This helps to avoid error cases where ID is not actually set.
				// For those, we have further specifications later.
				id = c.createDummyID(tb.(*testcase.T), resource)
			}

			return func(tb testing.TB, ctx context.Context) (ent Ent, found bool, err error) {
				return resource.FindByID(ctx, id)
			}
		},

		Specify: func(tb testing.TB) {
			t := tb.(*testcase.T)
			r := c.Subject(t)

			var ids []ID
			for i := 0; i < 12; i++ {
				ent := c.MakeEnt(t)
				Create[Ent, ID](t, r, c.Context(t), &ent)
				id, ok := extid.Lookup[ID](&ent)
				t.Must.True(ok, ErrIDRequired.Error())
				ids = append(ids, id)
			}

			t.Log("when no value stored that the query request")
			ctx := c.Context(t)
			_, ok, err := r.FindByID(c.Context(t), c.createNonActiveID(t, ctx, r))
			t.Must.Nil(err)
			t.Must.False(ok)

			t.Log("values returned")
			for _, id := range ids {
				ent, ok, err := r.FindByID(c.Context(t), id)
				t.Must.Nil(err)
				t.Must.True(ok)

				actualID, ok := extid.Lookup[ID](ent)
				t.Must.True(ok, "can't find ID in the returned value")
				t.Must.Equal(id, actualID)
			}
		},
	})
}

func (c findByID[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c findByID[Ent, ID]) Benchmark(b *testing.B) {
	s := testcase.NewSpec(b)
	r := c.Subject(b)

	ent := testcase.Let(s, func(t *testcase.T) *Ent {
		ptr := new(Ent)
		Create[Ent, ID](t, r, c.Context(t), ptr)
		return ptr
	}).EagerLoading(s)

	id := testcase.Let(s, func(t *testcase.T) ID {
		return HasID[Ent, ID](t, ent.Get(t))
	}).EagerLoading(s)

	s.Test(``, func(t *testcase.T) {
		_, _, err := r.FindByID(c.Context(t), id.Get(t))
		t.Must.Nil(err)
	})
}

func (c findByID[Ent, ID]) createNonActiveID(tb testing.TB, ctx context.Context, r FinderSubject[Ent, ID]) ID {
	tb.Helper()
	ent := c.MakeEnt(tb)
	ptr := &ent
	Create[Ent, ID](tb, r, ctx, ptr)
	id, _ := extid.Lookup[ID](ptr)
	Delete[Ent, ID](tb, r, ctx, ptr)
	return id
}

// findAll can return business entities from a given storage that implement it's test
// The "EntityTypeName" is a Empty struct for the specific entity (struct) type that should be returned.
//
// NewEntityForTest used only for testing and should not be provided outside of testing
type findAll[Ent, ID any] struct {
	Subject func(testing.TB) FinderSubject[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c findAll[Ent, ID]) String() string {
	return "Finder.FindAll"
}

func (c findAll[Ent, ID]) Spec(s *testcase.Spec) {
	resource := testcase.Let(s, func(t *testcase.T) FinderSubject[Ent, ID] {
		return c.Subject(t)
	})

	beforeAll := &sync.Once{}
	s.Before(func(t *testcase.T) {
		beforeAll.Do(func() {
			DeleteAll[Ent, ID](t, resource.Get(t), c.Context(t))
		})
	})

	s.Describe(`FindAll`, func(s *testcase.Spec) {
		var (
			ctx = testcase.Let(s, func(t *testcase.T) context.Context {
				return c.Context(t)
			})
			subject = func(t *testcase.T) frameless.Iterator[Ent] {
				return resource.Get(t).FindAll(ctx.Get(t))
			}
		)

		s.Before(func(t *testcase.T) {
			DeleteAll[Ent, ID](t, resource.Get(t), c.Context(t))
		})

		entity := testcase.Let(s, func(t *testcase.T) *Ent {
			ent := c.MakeEnt(t)
			return &ent
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				Create[Ent, ID](t, resource.Get(t), c.Context(t), entity.Get(t))
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
					contains[Ent](tb, entities, *entity.Get(t))
				})
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				othEntity := testcase.Let(s, func(t *testcase.T) *Ent {
					ent := c.MakeEnt(t)
					Create[Ent, ID](t, resource.Get(t), c.Context(t), &ent)
					return &ent
				}).EagerLoading(s)

				s.Then(`all entity will be fetched`, func(t *testcase.T) {
					Eventually.Assert(t, func(tb assert.It) {
						entities := c.findAllN(t, subject, 2)
						contains[Ent](tb, entities, *entity.Get(t))
						contains[Ent](tb, entities, *othEntity.Get(t))
					})
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				DeleteAll[Ent, ID](t, resource.Get(t), c.Context(t))
			})

			s.Then(`the iterator will have no result`, func(t *testcase.T) {
				count, err := iterators.Count(subject(t))
				t.Must.Nil(err)
				t.Must.Equal(0, count)
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.Context(t))
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
func (c findAll[Ent, ID]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c findAll[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c findByID[Ent, ID]) createDummyID(t *testcase.T, r FinderSubject[Ent, ID]) ID {
	ent := c.MakeEnt(t)
	ctx := c.Context(t)
	Create[Ent, ID](t, r, ctx, &ent)
	id := HasID[Ent, ID](t, &ent)
	Delete[Ent, ID](t, r, ctx, &ent)
	return id
}

func (c findAll[Ent, ID]) findAllN(t *testcase.T, subject func(t *testcase.T) frameless.Iterator[Ent], n int) []Ent {
	var entities []Ent
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
type QueryOne[Ent any] func(tb testing.TB, ctx context.Context) (ent Ent, found bool, err error)

type FindOne[Ent, ID any] struct {
	Subject func(testing.TB) FinderSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
	// MethodName is the name of the test subject QueryOne method of this contract specification.
	MethodName string
	// ToQuery takes an entity ptr and returns with a closure that has the knowledge about how to query on the Subject resource to find the entity.
	//
	// By convention, any preparation action that affect the Storage must take place prior to returning the closure.
	// The QueryOne closure should only have the Method call with the already mapped values.
	// ToQuery will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	ToQuery func(tb testing.TB, resource FinderSubject[Ent, ID], ent Ent) QueryOne[Ent]
	// Specify allow further specification describing for a given FindOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

func (c FindOne[Ent, ID]) String() string {
	return fmt.Sprintf(".%s", c.MethodName)
}

func (c FindOne[Ent, ID]) Spec(s *testcase.Spec) {
	var (
		ctx = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeCtx(t)
		})
		entity = testcase.Let(s, func(t *testcase.T) *Ent {
			ent := c.MakeEnt(t)
			return &ent
		})
		resource = testcase.Let(s, func(t *testcase.T) FinderSubject[Ent, ID] {
			return c.Subject(t)
		})
		query = testcase.Let(s, func(t *testcase.T) QueryOne[Ent] {
			t.Log(entity.Get(t))
			return c.ToQuery(t, resource.Get(t), *entity.Get(t))
		})
		subject = func(t *testcase.T) (Ent, bool, error) {
			return query.Get(t)(t, ctx.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		DeleteAll[Ent, ID](t, resource.Get(t), c.MakeCtx(t))
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Create[Ent, ID](t, resource.Get(t), c.MakeCtx(t), entity.Get(t))
			HasID[Ent, ID](t, entity.Get(t))
		})

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			ent, found, err := subject(t)
			t.Must.Nil(err)
			t.Must.True(found)
			t.Must.Equal(*entity.Get(t), ent)
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeCtx(t))
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
				ent := c.MakeEnt(t)
				Create[Ent, ID](t, resource.Get(t), c.MakeCtx(t), &ent)
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
			DeleteAll[Ent, ID](t, resource.Get(t), c.MakeCtx(t))
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

func (c FindOne[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c FindOne[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
