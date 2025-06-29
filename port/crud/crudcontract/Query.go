package crudcontract

import (
	"context"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/internal/spechelper"
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
	c := option.ToConfig[Config[ENT, ID]](opts)
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
			v := makeEntity(t, t.SkipNow, c, resource, func() ENT {
				ent := sub.Get(t).ExpectedEntity
				assert.NotEmpty(t, ent)
				return ent
			}, "QueryOne.Subject.ExpectedEntity")
			return v
		}).EagerLoading(s)

		s.Then(`the entity will be returned`, func(t *testcase.T) {
			got, found, err := act(t)
			t.Must.NoError(err)
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
				t.Must.NoError(err)
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
			t.Must.NoError(err)
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
			t.Must.NoError(err)
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
	Query func(ctx context.Context) iter.Seq2[ENT, error]
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
	c := option.ToConfig[Config[ENT, ID]](opts)

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
	act := func(t *testcase.T) iter.Seq2[ENT, error] {
		assert.NotNil(t, subject, "QueryMany subject has no MakeQuery value")
		return sub.Get(t).Query(ctx.Get(t))
	}

	s.When(`resource has entity that the query should return`, func(s *testcase.Spec) {
		includedEntity := testcase.Let(s, func(t *testcase.T) ENT {
			return MakeIncludedEntity(t)
		}).EagerLoading(s)

		s.Then(`the query will return the entity`, func(t *testcase.T) {
			t.Eventually(func(it *testcase.T) {
				ents, err := iterkit.CollectE(act(it))
				assert.NoError(t, err)
				assert.Contain(t, ents, includedEntity.Get(it))
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
				t.Eventually(func(t *testcase.T) {
					ents, err := iterkit.CollectE(act(t))
					assert.NoError(t, err)
					assert.Contain(t, ents, includedEntity.Get(t))
					assert.Contain(t, ents, additionalEntities.Get(t))
				})
			})

			s.Then("query execution can interlace between the same queries", func(t *testcase.T) { // multithreaded apps
				t.Eventually(func(t *testcase.T) {
					i1 := act(t)

					i1Next, i1Stop := iter.Pull2(i1)
					defer i1Stop()

					vsv1, err, ok := i1Next()
					assert.True(t, ok)
					assert.NoError(t, err)

					vs2, err := iterkit.CollectE(act(t))
					assert.NoError(t, err)
					assert.Contain(t, vs2, includedEntity.Get(t))
					assert.Contain(t, vs2, additionalEntities.Get(t))

					var vs1 []ENT
					vs1 = append(vs1, vsv1)
					for {
						v, err, ok := i1Next()
						if !ok {
							break
						}
						assert.NoError(t, err)
						vs1 = append(vs1, v)
					}

					vs1 = append(vs1, vsv1)
					assert.Contain(t, vs1, includedEntity.Get(t))
					assert.Contain(t, vs1, additionalEntities.Get(t))
				})
			})

			if subject, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
				s.Then("query execution can interlace with FindByID", func(t *testcase.T) { // multithreaded apps
					t.Eventually(func(t *testcase.T) {
						itr := act(t)

						probes := t.Random.IntBetween(3, 5)

						for value, err := range itr {
							assert.NoError(t, err)

							if probes == 0 {
								break
							}
							probes--

							ent, found, err := subject.FindByID(c.MakeContext(t), c.IDA.Get(value))
							assert.NoError(t, err)
							assert.True(t, found, "expected that FindByID will able to retrieve a value for the given ID")
							assert.Equal(t, value, ent)
						}
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
					ents, err := iterkit.CollectE(act(t))
					assert.NoError(t, err)
					assert.Contain(t, ents, includedEntity.Get(t))
					assert.NotContain(t, ents, othEnt.Get(t))
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
			gotErr := shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
				return act(t)
			})
			assert.ErrorIs(t, gotErr, ctx.Get(t).Err())
		})
	})

	return s.AsSuite(methodName)
}
