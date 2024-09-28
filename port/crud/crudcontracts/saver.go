package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Saver[ENT, ID any](subject crud.Saver[ENT], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[ENT, ID]](opts)

	s.Describe(`.Save`, func(s *testcase.Spec) {
		var (
			ctx = testcase.Let(s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})
			ptr = testcase.Let[*ENT](s, func(t *testcase.T) *ENT {
				v := c.MakeEntity(t)
				t.Cleanup(func() { tryDelete(t, c, subject, ctx.Get(t), v) })
				return &v
			})
		)
		act := func(t *testcase.T) error {
			return subject.Save(ctx.Get(t), ptr.Get(t))
		}

		s.When(`entity absent from the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, ok := lookupID[ID](c, *ptr.Get(t))
				if !ok {
					return
				}
				shouldAbsent[ENT, ID](t, c, subject, c.MakeContext(t), id)
			})

			s.Then(`it will be created`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				entID, ok := lookupID[ID](c, *ptr.Get(t))
				t.Must.True(ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					assert.NoError(t, err)
					assert.True(t, found, "expected to find the newly upserted entity")
					assert.Equal(t, got, *ptr.Get(t))
				})
			})
		})

		s.When(`entity has an ext id that no longer points to a findable record`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				if _, ok := lookupID[ID](c, *ptr.Get(t)); ok {
					return // OK, ID found
				}
				ctx := c.MakeContext(t)
				assert.NoError(t, subject.Save(ctx, ptr.Get(t)))
				shouldDelete(t, c, subject, ctx, *ptr.Get(t))
			})

			s.Then(`it will be created`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				entID, ok := lookupID[ID](c, *ptr.Get(t))
				t.Must.True(ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					t.Must.Nil(err)
					t.Must.True(found, `entity was expected to be stored`)
					t.Must.Equal(*ptr.Get(t), got)
				})
			})
		})

		s.When(`entity is present already in the resource`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) *ENT {
				v := ptr.Super(t)
				assert.NoError(t, subject.Save(c.MakeContext(t), v))
				return v
			}).EagerLoading(s)

			s.Then(`it will be updated with the new version`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				entID, ok := lookupID[ID](c, *ptr.Get(t))
				t.Must.True(ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					t.Must.Nil(err)
					t.Must.True(found, `entity was expected to be stored`)
					t.Must.Equal(*ptr.Get(t), got)
				})
			})
		})

		s.When(`entity is a newer version compared to the stored one`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) *ENT {
				v := ptr.Super(t)
				assert.NoError(t, subject.Save(c.MakeContext(t), v))
				changeENT(t, c, v) // change entity to represent an update state
				return v
			}).EagerLoading(s)

			s.Then(`it will be updated with the new version`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				entID, ok := lookupID[ID](c, *ptr.Get(t))
				t.Must.True(ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					t.Must.Nil(err)
					t.Must.True(found, `entity was expected to be stored`)
					t.Must.Equal(*ptr.Get(t), got)
				})
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				allFinder, ok := subject.(crud.AllFinder[ENT])
				if !ok {
					t.Skipf("unable to continue with the test, crud.AllFinder is not implemented in %T", subject)
				}

				iter, err := allFinder.FindAll(ctx.Get(t))
				assert.NoError(t, err)
				initialCount, err := iterators.Count(iter)
				assert.NoError(t, err)

				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					iter, err = allFinder.FindAll(ctx.Get(t))
					assert.NoError(t, err)

					count, err := iterators.Count(iter)
					assert.NoError(t, err)
					assert.Equal(t, initialCount, count)
				})
			})
		})

	})

	return s.AsSuite("crud.Save")
}
