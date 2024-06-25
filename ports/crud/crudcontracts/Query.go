package crudcontracts

import (
	"context"
	"reflect"
	"testing"

	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/option"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// Query takes an entity value and returns with a closure that has the knowledge about how to query resource to find passed entity.
//
// By convention, any preparation action that affect the resource must take place prior to returning the closure.
// The QueryOneFunc closure should only have the Method call with the already mapped values.
// Query will be evaluated in the beginning of the testing,
// and executed after all the test Context preparation is done.
//
// The func signature for Query is the generic representation of a query that meant to find one result.
// It is really similar to resources.Finder#FindByID,
// with the exception that the closure meant to know the query method name on the subject and the inputs it requires.
type QueryOneFunc[Entity any] func(ctx context.Context, ent Entity) (_ Entity, found bool, _ error)

func QueryOne[Entity, ID any](subject any, query QueryOneFunc[Entity], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext()
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(c.MakeEntity(t))
		})
	)
	act := func(t *testcase.T) (Entity, bool, error) {
		return query(context.WithValue(ctx.Get(t), TestingTBContextKey{}, t), *ptr.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(), subject)
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		ptr.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(ensureExistingEntity(t, c, subject))
		})

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			ent, found, err := act(t)
			t.Must.Nil(err)
			t.Must.True(found)
			t.Must.Equal(*ptr.Get(t), ent)
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext())
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
				ensureExistingEntity(t, c, subject, *ptr.Get(t))
			})

			s.Then(`still the correct entity is returned`, func(t *testcase.T) {
				ent, found, err := act(t)
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(*ptr.Get(t), ent)
			})
		})
	})

	s.When(`no entity is saved in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !spechelper.TryCleanup(t, c.MakeContext(), subject) {
				t.Skip("unable to clean resource")
			}
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := act(t)
			t.Must.Nil(err)
			t.Must.False(found)
		})
	})

	return s.AsSuite("QueryOne")
}

type QueryManyFunc[Entity any] func(ctx context.Context) iterators.Iterator[Entity]

func QueryMany[Entity, ID any](
	subject any,
	// Query which is the subject of this contract,
	query QueryManyFunc[Entity],
	// IncludedEntity return an entity that is matched by the QueryManyFunc.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	IncludedEntity func(tb testing.TB) Entity,
	// MakeExcludedEntity is an optional property,
	// that could be used ensure the query returns only the expected values.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	ExcludedEntity func(tb testing.TB) Entity,
	opts ...Option[Entity, ID],
) contract.Contract {

	s := testcase.NewSpec(nil)
	c := option.Use[Config[Entity, ID]](opts)

	var MakeIncludedEntity = func(tb testing.TB) Entity {
		assert.NotNil(tb, IncludedEntity, "MakeIncludedEntity is mandatory for QueryMany")
		return makeEntity[Entity, ID](tb, c, subject, IncludedEntity)
	}

	s.Before(func(t *testcase.T) {
		assert.NotNil(t, subject, "subject value is mandatory for QueryMany")
	})

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(), subject)
	})

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext()
		})
	)
	act := func(t *testcase.T) iterators.Iterator[Entity] {
		assert.NotNil(t, query, "QueryMany subject has no MakeQuery value")
		return query(context.WithValue(ctx.Get(t), TestingTBContextKey{}, t))
	}

	s.When(`resource has entity that the query should return`, func(s *testcase.Spec) {
		includedEntity := testcase.Let(s, func(t *testcase.T) Entity {
			return MakeIncludedEntity(t)
		}).EagerLoading(s)

		s.Then(`the query will return the entity`, func(t *testcase.T) {
			t.Eventually(func(it *testcase.T) {
				ents, err := iterators.Collect(act(t))
				it.Must.NoError(err)
				it.Must.Contain(ents, includedEntity.Get(t))
			})
		})

		s.And(`another similar entities are saved in the resource`, func(s *testcase.Spec) {
			additionalEntities := testcase.Let(s, func(t *testcase.T) (ents []Entity) {
				t.Random.Repeat(1, 3, func() {
					ents = append(ents, MakeIncludedEntity(t))
				})
				for i, a := range ents {
					for j, b := range ents {
						if i == j {
							continue
						}
						if !reflect.DeepEqual(a, b) {
							t.Skip("skipping test as it requires that IncludeEntity produces unique entities (at least unique ID)")
						}
					}
				}
				return ents
			}).EagerLoading(s)

			s.Then(`both entity is returned`, func(t *testcase.T) {
				t.Eventually(func(it *testcase.T) {
					ents, err := iterators.Collect(act(t))
					it.Must.NoError(err)
					t.Must.Contain(ents, includedEntity.Get(t))
					t.Must.Contain(ents, additionalEntities.Get(t))
				})
			})

			s.Then("query execution can interlace between the same queries", func(t *testcase.T) { // multithreaded apps
				t.Eventually(func(it *testcase.T) {
					i1 := act(t)
					it.Cleanup(func() { _ = i1.Close() })
					it.Must.True(i1.Next())
					it.Must.NoError(i1.Err())
					vsv1 := i1.Value()

					i2 := act(t)
					it.Cleanup(func() { _ = i2.Close() })

					vs2, err := iterators.Collect(i2)
					it.Must.NoError(err)
					it.Must.Contain(vs2, includedEntity.Get(t))
					it.Must.Contain(vs2, additionalEntities.Get(t))

					vs1, err := iterators.Collect(i1)
					it.Must.NoError(err)
					vs1 = append(vs1, vsv1)
					it.Must.Contain(vs1, includedEntity.Get(t))
					it.Must.Contain(vs1, additionalEntities.Get(t))
				})
			})

			if subject, ok := subject.(crud.ByIDFinder[Entity, ID]); ok {
				s.Then("query execution can interlace with FindByID", func(t *testcase.T) { // multithreaded apps
					t.Eventually(func(it *testcase.T) {
						iter := act(t)
						defer func() { it.Must.NoError(iter.Close()) }()
						for iter.Next() {
							value := iter.Value()

							id, ok := extid.Lookup[ID](value)
							it.Must.True(ok)

							ent, found, err := subject.FindByID(c.MakeContext(), id)
							it.Must.NoError(err)
							it.Must.True(found)
							it.Must.Equal(value, ent)
						}
						it.Must.NoError(iter.Err())
					})
				})
			}
		})

		if ExcludedEntity != nil {
			s.And(`an entity that does not match our query requirements is saved in the resource`, func(s *testcase.Spec) {
				othEnt := testcase.Let(s, func(t *testcase.T) Entity {
					return makeEntity[Entity, ID](t, c, subject, ExcludedEntity)
				}).EagerLoading(s)

				s.Then(`only the matching entity is returned`, func(t *testcase.T) {
					t.Eventually(func(it *testcase.T) {
						ents, err := iterators.Collect(act(t))
						it.Must.NoError(err)
						it.Must.Contain(ents, includedEntity.Get(t))
						it.Must.NotContain(ents, othEnt.Get(t))
					})
				})
			})
		}
	})

	s.When(`ctx is done and has an error`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with context error`, func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
			t.Must.ErrorIs(ctx.Get(t).Err(), err)
			t.Must.Empty(vs)
		})
	})

	return s.AsSuite("QueryMany")
}
