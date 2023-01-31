package crudcontracts

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

// QueryOneFunc is the generic representation of a query that meant to find one result.
// It is really similar to resources.Finder#FindByID,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOneFunc is generated through ToQuery factory function in QueryOne resource contract specification.
type QueryOneFunc[Entity any] func(tb testing.TB, ctx context.Context, ent Entity) (_ Entity, found bool, _ error)

type QueryOne[Entity, ID any] struct {
	MakeSubject func(testing.TB) QueryOneSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	// QueryName is the name of the test subject QueryOneFunc method of this contract specification.
	QueryName string
	// Specify allow further specification describing for a given QueryOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

type QueryOneSubject[Entity, ID any] struct {
	// Resource is the resource that can contain entities.
	Resource spechelper.CRD[Entity, ID]
	// Query takes an entity ptr and returns with a closure that has the knowledge about how to query on the MakeSubject resource to find the entity.
	//
	// By convention, any preparation action that affect the resource must take place prior to returning the closure.
	// The QueryOneFunc closure should only have the Method call with the already mapped values.
	// Query will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	Query QueryOneFunc[Entity]
}

func (c QueryOne[Entity, ID]) Name() string {
	if c.QueryName != "" {
		return c.QueryName
	}
	return fmt.Sprintf(`QueryOne[%T, %T]`, *new(Entity), *new(ID))
}

func (c QueryOne[Entity, ID]) Spec(s *testcase.Spec) {
	var (
		ctx     = testcase.Let(s, spechelper.ToLet(c.MakeContext))
		entity  = testcase.Let(s, spechelper.ToLetPtr(c.MakeEntity))
		subject = testcase.Let(s, spechelper.ToLet(c.MakeSubject))
		act     = func(t *testcase.T) (Entity, bool, error) {
			return subject.Get(t).Query(t, ctx.Get(t), *entity.Get(t))
		}
	)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), subject.Get(t))
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), entity.Get(t))
		})

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			ent, found, err := act(t)
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
				_, found, err := act(t)
				t.Must.ErrorIs(context.Canceled, err)
				t.Must.False(found)
			})
		})

		s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ent := c.MakeEntity(t)
				crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), &ent)
			})

			s.Then(`still the correct entity is returned`, func(t *testcase.T) {
				ent, found, err := act(t)
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(*entity.Get(t), ent)
			})
		})
	})

	s.When(`no entity is saved in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			spechelper.TryCleanup(t, c.MakeContext(t), subject.Get(t))
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := act(t)
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

func (c QueryOne[Entity, ID]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c QueryOne[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

// QueryManyFunc is the generic representation of a query that meant to find many result.
// It is really similar to resources.Finder#FindAll,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOneFunc is generated through ToQuery factory function in QueryOne resource contract specification.
type QueryManyFunc[Entity any] func(tb testing.TB, ctx context.Context) iterators.Iterator[Entity]

type QueryMany[Entity, ID any] struct {
	MakeSubject func(testing.TB) QueryManySubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	// QueryName is the name of the test subject QueryOneFunc method of this contract specification.
	QueryName string
	// MakeIncludedEntity must return an entity that is matched by the QueryManyFunc
	MakeIncludedEntity func(testing.TB) Entity
	// MakeExcludedEntity [OPTIONAL] is an optional property,
	// that could be used ensure the query returns only the expected values.
	MakeExcludedEntity func(testing.TB) Entity
	// Specify allow further specification describing for a given QueryOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

type QueryManySubject[Entity, ID any] struct {
	// Resource is the resource that can contain entities.
	Resource spechelper.CRD[Entity, ID]
	// MakeQuery is the function that knows how to build up the QueryManyFunc.
	MakeQuery QueryManyFunc[Entity]
}

func (c QueryMany[Entity, ID]) Name() string {
	if c.QueryName != "" {
		return c.QueryName
	}
	return fmt.Sprintf(`QueryMany[%T, %T]`, *new(Entity), *new(ID))
}

func (c QueryMany[Entity, ID]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) QueryManySubject[Entity, ID] {
		subject := c.MakeSubject(t)
		t.Must.NotNil(subject.Resource, "QueryManySubject has no Resource value")
		t.Must.NotNil(subject.MakeQuery, "QueryManySubject has no MakeQuery value")
		return subject
	})

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), subject.Get(t).Resource)
	})

	var (
		ctx = testcase.Let(s, spechelper.ToLet(c.MakeContext))
	)
	act := func(t *testcase.T) iterators.Iterator[Entity] {
		return subject.Get(t).MakeQuery(t, ctx.Get(t))
	}

	s.When(`resource has entity that the query should return`, func(s *testcase.Spec) {
		includedEntity := testcase.Let(s, func(t *testcase.T) Entity {
			ent := c.MakeIncludedEntity(t)
			crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), &ent)
			return ent
		}).EagerLoading(s)

		s.Then(`the query will return the entity`, func(t *testcase.T) {
			t.Eventually(func(it assert.It) {
				ents, err := iterators.Collect(act(t))
				it.Must.NoError(err)
				it.Must.Contain(ents, includedEntity.Get(t))
			})
		})

		s.And(`another similar entities are saved in the resource`, func(s *testcase.Spec) {
			additionalEntities := testcase.Let(s, func(t *testcase.T) (ents []Entity) {
				t.Random.Repeat(1, 3, func() {
					ent := c.MakeIncludedEntity(t)
					crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), &ent)
					ents = append(ents, ent)
				})
				return ents
			}).EagerLoading(s)

			s.Then(`both entity is returned`, func(t *testcase.T) {
				t.Eventually(func(it assert.It) {
					ents, err := iterators.Collect(act(t))
					it.Must.NoError(err)
					t.Must.Contain(ents, includedEntity.Get(t))
					t.Must.Contain(ents, additionalEntities.Get(t))
				})
			})
		})

		if c.MakeExcludedEntity != nil {
			s.And(`an entity that does not match our query requirements is saved in the resource`, func(s *testcase.Spec) {
				othEnt := testcase.Let(s, func(t *testcase.T) Entity {
					ent := c.MakeExcludedEntity(t)
					crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), &ent)
					return ent
				}).EagerLoading(s)

				s.Then(`only the matching entity is returned`, func(t *testcase.T) {
					t.Eventually(func(it assert.It) {
						ents, err := iterators.Collect(act(t))
						it.Must.NoError(err)
						t.Must.Contain(ents, includedEntity.Get(t))
						t.Must.NotContain(ents, othEnt.Get(t))
					})
				})
			})
		}
	})

	s.When(`ctx is done and has an error`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with context error`, func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
			t.Must.ErrorIs(ctx.Get(t).Err(), err)
			t.Must.Empty(vs)
		})
	})

	if c.Specify != nil {
		s.Test(`Specify`, func(t *testcase.T) { c.Specify(t) })
	}
}

func (c QueryMany[Entity, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c QueryMany[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
