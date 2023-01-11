package crudcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

// QueryOneFunc is the generic representation of a query that meant to find one result.
// It is really similar to resources.Finder#FindByID,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOneFunc is generated through ToQuery factory function in FindOne resource contract specification.
type QueryOneFunc[Entity any] func(tb testing.TB, ctx context.Context) (ent Entity, found bool, err error)

// QueryManyFunc is the generic representation of a query that meant to find many result.
// It is really similar to resources.Finder#FindAll,
// with the exception that the closure meant to know the query method name on the subject
// and the inputs it requires.
//
// QueryOneFunc is generated through ToQuery factory function in FindOne resource contract specification.
type QueryManyFunc[Entity any] func(tb testing.TB, ctx context.Context) iterators.Iterator[Entity]

type QueryMany[Entity, ID any] struct {
	MakeSubject func(testing.TB) QueryManySubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	// MakeIncludedEntity must return an entity that is matched by the QueryManyFunc
	MakeIncludedEntity func(testing.TB) Entity
	// MakeExcludedEntity [OPTIONAL] is an optional property,
	// that could be used ensure the query returns only the expected values.
	MakeExcludedEntity func(testing.TB) Entity
}

type QueryManySubject[Entity, ID any] struct {
	// Resource is the resource that can contain entities.
	Resource spechelper.CRD[Entity, ID]
	// MakeQuery is the function that knows how to build up the QueryManyFunc.
	MakeQuery QueryManyFunc[Entity]
}

func (c QueryMany[Entity, ID]) Name() string {
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
				for i, m := 0, t.Random.IntB(3, 7); i < m; i++ {
					ent := c.MakeIncludedEntity(t)
					crudtest.Create[Entity, ID](t, subject.Get(t).Resource, c.MakeContext(t), &ent)
					ents = append(ents, ent)
				}
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
}

func (c QueryMany[Entity, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c QueryMany[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
