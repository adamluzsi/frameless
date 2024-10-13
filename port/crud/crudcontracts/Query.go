package crudcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
	"go.llib.dev/testcase/sandbox"
)

type QueryOneSubject[ENT any] struct {
	// Query creates a query that expected to find the ExpectedEntity
	//
	// By convention, any preparation action that affect the resource must take place prior to returning the closure.
	// The QueryOneFunc closure should only have the Method call with the already mapped values.
	// Query will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	//
	// The func signature for Query is the generic representation of a query that meant to find one result.
	// It is really similar to resources.Finder#FindByID,
	// with the exception that the closure meant to know the query method name on the subject and the inputs it requires.
	Query func(ctx context.Context) (_ ENT, found bool, _ error)
	// ExpectedEntity is an entity that is matched by the Query.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	ExpectedEntity ENT
	// MakeExcludedEntity is an optional property,
	// that could be used ensure the query returns only the expected values.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	ExcludedEntity func() ENT
}

func QueryOne[ENT, ID any](
	resource any,
	methodName string,
	subject func(tb testing.TB) QueryOneSubject[ENT],
	opts ...Option[ENT, ID],
) contract.Contract {
	c := option.Use[Config[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	sub := testcase.Let(s, func(t *testcase.T) QueryOneSubject[ENT] {
		return subject(t)
	})

	var (
		Context = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
	)
	act := func(t *testcase.T) (ENT, bool, error) {
		return sub.Get(t).Query(Context.Get(t))
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), resource)
	})

	s.When(`entity was present in the resource`, func(s *testcase.Spec) {
		ent := testcase.Let(s, func(t *testcase.T) ENT {
			v := makeEntity(t, t.FailNow, c, resource, func() ENT {
				ent := sub.Get(t).ExpectedEntity
				assert.NotEmpty(t, ent)
				return ent
			}, "QueryOne.Subject.ExpectedEntity")
			shouldStore(t, c, resource, &v)
			return v
		}).EagerLoading(s)

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			got, found, err := act(t)
			t.Must.Nil(err)
			t.Must.True(found)
			t.Must.Equal(got, ent.Get(t))
		})

		s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				assert.Error(t, ctx.Err())
				return ctx
			})

			s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
				_, found, err := act(t)
				t.Must.ErrorIs(context.Canceled, err)
				t.Must.False(found)
			})
		})

		s.And(`more entity is saved in the resource as well`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ensureExistingEntity(t, c, resource, sub.Get(t).ExcludedEntity, ent.Get(t))
			})

			s.Then(`still the correct entity is returned`, func(t *testcase.T) {
				got, found, err := act(t)
				t.Must.Nil(err)
				t.Must.True(found)
				t.Must.Equal(got, ent.Get(t))
			})
		})
	})

	s.When(`no entity is saved in the resource`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			if !spechelper.TryCleanup(t, c.MakeContext(t), resource) {
				t.Skip("unable to clean resource")
			}
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := act(t)
			t.Must.Nil(err)
			t.Must.False(found)
		})
	})

	s.When(`unrelated entities are stored in the database`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			var ents []ENT
			t.Random.Repeat(3, 7, func() {
				ent := ensureExistingEntity(t, c, resource, sub.Get(t).ExcludedEntity, ents...)
				ents = append(ents, ent)
			})
		})

		s.Then(`it will have no result`, func(t *testcase.T) {
			_, found, err := act(t)
			t.Must.Nil(err)
			t.Must.False(found)
		})
	})

	return s.AsSuite(methodName)
}

type QueryManySubject[ENT any] struct {
	// Query creates a query that expected to find the ExpectedEntity
	//
	// By convention, any preparation action that affect the resource must take place prior to returning the closure.
	// The QueryOneFunc closure should only have the Method call with the already mapped values.
	// Query will be evaluated in the beginning of the testing,
	// and executed after all the test Context preparation is done.
	//
	// The func signature for Query is the generic representation of a query that meant to find one result.
	// It is really similar to resources.Finder#FindByID,
	// with the exception that the closure meant to know the query method name on the subject and the inputs it requires.
	Query func(ctx context.Context) (iterators.Iterator[ENT], error)
	// IncludedEntity return an entity that is matched by the QueryManyFunc.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	IncludedEntity func() ENT
	// MakeExcludedEntity is an optional property,
	// that could be used ensure the query returns only the expected values.
	// If subject doesn't support Creator, then it should be present in the subject resource.
	ExcludedEntity func() ENT
}

func QueryMany[ENT, ID any](
	resource any,
	methodName string,
	subject func(testing.TB) QueryManySubject[ENT],
	opts ...Option[ENT, ID],
) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[ENT, ID]](opts)

	sub := testcase.Let(s, func(t *testcase.T) QueryManySubject[ENT] {
		return subject(t)
	})

	var MakeIncludedEntity = func(t *testcase.T) ENT {
		assert.NotNil(t, sub.Get(t).IncludedEntity, "MakeIncludedEntity is mandatory for QueryMany")
		return makeEntity[ENT, ID](t, t.FailNow, c, resource, sub.Get(t).IncludedEntity, "QueryMany IncludedEntity argument")
	}

	s.Before(func(t *testcase.T) {
		assert.NotNil(t, resource, "subject value is mandatory for QueryMany")
	})

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), resource)
	})

	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
	)
	act := func(t *testcase.T) (iterators.Iterator[ENT], error) {
		assert.NotNil(t, subject, "QueryMany subject has no MakeQuery value")
		return sub.Get(t).Query(ctx.Get(t))
	}

	s.When(`resource has entity that the query should return`, func(s *testcase.Spec) {
		includedEntity := testcase.Let(s, func(t *testcase.T) ENT {
			return MakeIncludedEntity(t)
		}).EagerLoading(s)

		s.Then(`the query will return the entity`, func(t *testcase.T) {
			t.Eventually(func(it *testcase.T) {
				iter, err := act(it)
				assert.NoError(it, err)
				ents, err := iterators.Collect(iter)
				it.Must.NoError(err)
				it.Must.Contain(ents, includedEntity.Get(t))
			})
		})

		s.And(`another similar entities are saved in the resource`, func(s *testcase.Spec) {
			additionalEntities := testcase.Let(s, func(t *testcase.T) (ents []ENT) {
				sandbox.Run(func() {
					ents = random.Slice(t.Random.IntB(1, 3),
						func() ENT { return MakeIncludedEntity(t) },
						random.UniqueValues)
				}).OnNotOK(func() {
					t.Skip("skipping this test as it requires that MakeIncludedEntity creates unique entities")
				})
				return ents
			}).EagerLoading(s)

			s.Then(`both entity is returned`, func(t *testcase.T) {
				t.Eventually(func(it *testcase.T) {
					iter, err := act(it)
					assert.NoError(it, err)
					ents, err := iterators.Collect(iter)
					it.Must.NoError(err)
					t.Must.Contain(ents, includedEntity.Get(t))
					t.Must.Contain(ents, additionalEntities.Get(t))
				})
			})

			s.Then("query execution can interlace between the same queries", func(t *testcase.T) { // multithreaded apps
				t.Eventually(func(it *testcase.T) {
					i1, err := act(it)
					assert.NoError(it, err)

					it.Cleanup(func() { _ = i1.Close() })
					it.Must.True(i1.Next())
					it.Must.NoError(i1.Err())
					vsv1 := i1.Value()

					i2, err := act(it)
					assert.NoError(it, err)
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

			if subject, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
				s.Then("query execution can interlace with FindByID", func(t *testcase.T) { // multithreaded apps
					t.Eventually(func(it *testcase.T) {
						iter, err := act(it)
						assert.NoError(it, err)
						iter = iterators.Head(iter, t.Random.IntBetween(3, 5))
						defer func() { it.Must.NoError(iter.Close()) }()

						for iter.Next() {
							value := iter.Value()

							id, ok := lookupID[ID](c, value)
							it.Must.True(ok, "expected that value has an external ID reference")

							ent, found, err := subject.FindByID(c.MakeContext(t), id)
							it.Must.NoError(err)
							it.Must.True(found, "expected that FindByID will able to retrieve a value for the given ID")
							it.Must.Equal(value, ent)
						}
						it.Must.NoError(iter.Err())
					})
				})
			}
		})

		s.And(`an entity that does not match our query requirements is saved in the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				if sub.Get(t).ExcludedEntity == nil {
					t.Skip("skipping test, as ExcludedEntity is not supplied")
				}
			})

			othEnt := testcase.Let(s, func(t *testcase.T) ENT {
				return makeEntity[ENT, ID](t, t.FailNow, c, resource, sub.Get(t).ExcludedEntity, "QueryMany ExcludedEntity argument")
			}).EagerLoading(s)

			s.Then(`only the matching entity is returned`, func(t *testcase.T) {
				t.Eventually(func(t *testcase.T) {
					iter, err := act(t)
					assert.NoError(t, err)
					ents, err := iterators.Collect(iter)
					t.Must.NoError(err)
					t.Must.Contain(ents, includedEntity.Get(t))
					t.Must.NotContain(ents, othEnt.Get(t))
				})
			})
		})
	})

	s.When(`ctx is done and has an error`, func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with context error`, func(t *testcase.T) {
			gotErr := shouldIterEventuallyError(t, func() (iterators.Iterator[ENT], error) {
				return act(t)
			})
			assert.ErrorIs(t, gotErr, ctx.Get(t).Err())
		})
	})

	return s.AsSuite(methodName)
}
