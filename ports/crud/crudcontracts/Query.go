package crudcontracts

import (
	"context"
	"go.llib.dev/frameless/ports/crud/extid"
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

type QueryOne[Entity, ID any] func(testing.TB) QueryOneSubject[Entity, ID]

type QueryOneSubject[Entity, ID any] struct {
	// Resource is the resource that can contain entities.
	Resource spechelper.CRD[Entity, ID]
	// Name is the name of the test subject QueryOneFunc method of this contract specification.
	Name string
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
	Query       func(ctx context.Context, ent Entity) (_ Entity, found bool, _ error)
	MakeContext func() context.Context
	MakeEntity  func() Entity
	// Specify allow further specification describing for a given QueryOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

func (c QueryOne[Entity, ID]) Spec(s *testcase.Spec) {
	subject := let.With[QueryOneSubject[Entity, ID]](s, (func(tb testing.TB) QueryOneSubject[Entity, ID])(c))

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return subject.Get(t).MakeContext()
		})
		ptr = testcase.Let(s, func(t *testcase.T) *Entity {
			return pointer.Of(subject.Get(t).MakeEntity())
		})
	)
	act := func(t *testcase.T) (Entity, bool, error) {
		return subject.Get(t).Query(ctx.Get(t), *ptr.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, subject.Get(t).MakeContext(), subject.Get(t))
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			crudtest.Create[Entity, ID](t, subject.Get(t).Resource, subject.Get(t).MakeContext(), ptr.Get(t))
		})

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			ent, found, err := act(t)
			t.Must.Nil(err)
			t.Must.True(found)
			t.Must.Equal(*ptr.Get(t), ent)
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(subject.Get(t).MakeContext())
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
				ent := subject.Get(t).MakeEntity()
				crudtest.Create[Entity, ID](t, subject.Get(t).Resource, subject.Get(t).MakeContext(), &ent)
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
			spechelper.TryCleanup(t, subject.Get(t).MakeContext(), subject.Get(t))
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := act(t)
			t.Must.Nil(err)
			t.Must.False(found)
		})
	})

	s.Test(`Specify`, func(t *testcase.T) {
		Specify := subject.Get(t).Specify
		if Specify == nil {
			t.Skip("no Specify supplied")
		}
		Specify(t)
	})
}

func (c QueryOne[Entity, ID]) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c QueryOne[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }

type QueryMany[Entity, ID any] func(testing.TB) QueryManySubject[Entity, ID]

type QueryManySubject[Entity, ID any] struct {
	// Resource is the resource that can contain entities.
	Resource spechelper.CRD[Entity, ID]
	// Query is the function that knows how to execute a query that will match MakeIncludedEntity.
	//
	// QueryManyFunc is the generic representation of a query that meant to find many result.
	// It is really similar to resources.Finder#FindAll,
	// with the exception that the closure meant to know the query method name on the subject
	// and the inputs it requires.
	Query       func(ctx context.Context) iterators.Iterator[Entity]
	MakeContext func() context.Context
	// MakeIncludedEntity must return an entity that is matched by the QueryManyFunc
	MakeIncludedEntity func() Entity
	// MakeExcludedEntity [OPTIONAL] is an optional property,
	// that could be used ensure the query returns only the expected values.
	MakeExcludedEntity func() Entity
	// Specify allow further specification describing for a given QueryOne query function.
	// If none specified, this field will be ignored
	Specify func(testing.TB)
}

func (c QueryMany[Entity, ID]) Spec(s *testcase.Spec) {
	subject := testcase.Let(s, func(t *testcase.T) QueryManySubject[Entity, ID] {
		subject := c(t)
		t.Must.NotNil(subject.Resource, "QueryManySubject has no Resource value")
		t.Must.NotNil(subject.Query, "QueryManySubject has no MakeQuery value")
		return subject
	})

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, subject.Get(t).MakeContext(), subject.Get(t).Resource)
	})

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return subject.Get(t).MakeContext()
		})
	)
	act := func(t *testcase.T) iterators.Iterator[Entity] {
		return subject.Get(t).Query(ctx.Get(t))
	}

	s.When(`resource has entity that the query should return`, func(s *testcase.Spec) {
		includedEntity := testcase.Let(s, func(t *testcase.T) Entity {
			ent := subject.Get(t).MakeIncludedEntity()
			crudtest.Create[Entity, ID](t, subject.Get(t).Resource, subject.Get(t).MakeContext(), &ent)
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
					ent := subject.Get(t).MakeIncludedEntity()
					crudtest.Create[Entity, ID](t, subject.Get(t).Resource, subject.Get(t).MakeContext(), &ent)
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

			s.Then("query execution can interlace between the same queries", func(t *testcase.T) { // multithreaded apps
				t.Eventually(func(it assert.It) {
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

			s.Then("query execution can interlace with FindByID", func(t *testcase.T) { // multithreaded apps
				t.Eventually(func(it assert.It) {
					iter := act(t)
					defer func() { it.Must.NoError(iter.Close()) }()
					for iter.Next() {
						value := iter.Value()

						id, ok := extid.Lookup[ID](value)
						it.Must.True(ok)

						ent, found, err := subject.Get(t).Resource.FindByID(subject.Get(t).MakeContext(), id)
						it.Must.NoError(err)
						it.Must.True(found)
						it.Must.Equal(value, ent)
					}
					it.Must.NoError(iter.Err())
				})
			})
		})

		s.And(`an entity that does not match our query requirements is saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				if subject.Get(t).MakeExcludedEntity == nil {
					t.Skip("MakeExcludedEntity is not supplied")
				}
			})

			othEnt := testcase.Let(s, func(t *testcase.T) Entity {
				ent := subject.Get(t).MakeExcludedEntity()
				crudtest.Create[Entity, ID](t, subject.Get(t).Resource, subject.Get(t).MakeContext(), &ent)
				return ent
			}).EagerLoading(s)

			s.Then(`only the matching entity is returned`, func(t *testcase.T) {
				t.Eventually(func(it assert.It) {
					ents, err := iterators.Collect(act(t))
					it.Must.NoError(err)
					it.Must.Contain(ents, includedEntity.Get(t))
					it.Must.NotContain(ents, othEnt.Get(t))
				})
			})
		})
	})

	s.When(`ctx is done and has an error`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(subject.Get(t).MakeContext())
			cancel()
			return ctx
		})

		s.Then(`it expected to return with context error`, func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
			t.Must.ErrorIs(ctx.Get(t).Err(), err)
			t.Must.Empty(vs)
		})
	})

	s.Test(`Specify`, func(t *testcase.T) {
		specify := subject.Get(t).Specify
		if specify == nil {
			t.Skip("no Specify supplied")
		}
		specify(t)
	})
}

func (c QueryMany[Entity, ID]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c QueryMany[Entity, ID]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
