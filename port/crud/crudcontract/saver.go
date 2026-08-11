package crudcontract

import (
	"context"
	"runtime"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func Saver[ENT, ID any](subject crud.Saver[ENT], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	byIDF, ByIDFinderOK := subject.(crud.ByIDFinder[ENT, ID])

	s.Describe(`.Save`, func(s *testcase.Spec) {
		var (
			ctx = testcase.Let(s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})
			ptr = testcase.Let[*ENT](s, func(t *testcase.T) *ENT {
				v := c.MakeEntity(t)
				t.Cleanup(func() { tryDelete(t, c, subject, v) })
				return &v
			})
		)
		act := func(t *testcase.T) error {
			return subject.Save(ctx.Get(t), ptr.Get(t))
		}

		s.When(`entity absent from the resource`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				id, ok := lookupNonZeroID[ID](c, *ptr.Get(t))
				if !ok {
					return
				}
				shouldAbsent[ENT, ID](t, c, subject, c.MakeContext(t), id)
			})

			s.Then(`it will be created`, func(t *testcase.T) {
				assert.Must(t).NoError(act(t))

				entID, ok := lookupNonZeroID[ID](c, *ptr.Get(t))
				assert.True(t, ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					assert.NoError(t, err)
					assert.True(t, found, "expected to find the newly upserted entity")
					assert.Equal(t, got, *ptr.Get(t))
				})
			})
		})

		// "Concurrent calls are race-free" is a happy-case concurrency check.
		// Each goroutine Saves a distinct entity concurrently.
		// The implementation must persist every entity without race.
		s.Test(`concurrent calls to Save are race-free`, func(t *testcase.T) {
			ctx := c.MakeContext(t)

			type pair struct{ ptr *ENT }
			var pairs []pair
			t.Random.Repeat(2, 5, func() {
				v := c.MakeEntity(t)
				pairs = append(pairs, pair{ptr: &v})
			})

			var ops []func()
			for i := range pairs {
				p := pairs[i]
				ops = append(ops, func() {
					assert.NoError(t, subject.Save(ctx, p.ptr))
				})
				t.Cleanup(func() {
					if v, ok := lookupNonZeroID[ID](c, *p.ptr); ok {
						tryDelete(t, c, subject, *p.ptr)
						_ = v
					}
				})
			}
			raceConcurrently(ops)

			if ByIDFinderOK {
				for _, p := range pairs {
					if id, ok := lookupNonZeroID[ID](c, *p.ptr); ok {
						c.Helper().IsPresent(t, byIDF, ctx, id)
					}
				}
			}
		})

		s.When(`entity has an ext id that no longer points to a findable record`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				if _, ok := lookupNonZeroID[ID](c, *ptr.Get(t)); ok {
					return // OK, ID found
				}
				ctx := c.MakeContext(t)
				assert.NoError(t, subject.Save(ctx, ptr.Get(t)))
				shouldDelete(t, c, subject, ctx, *ptr.Get(t))
			})

			s.Then(`it will be created`, func(t *testcase.T) {
				assert.Must(t).NoError(act(t))

				entID, ok := lookupNonZeroID[ID](c, *ptr.Get(t))
				assert.True(t, ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					assert.Must(t).NoError(err)
					assert.True(t, found, `entity was expected to be stored`)
					assert.Must(t).Equal(*ptr.Get(t), got)
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
				assert.Must(t).NoError(act(t))

				entID, ok := lookupNonZeroID[ID](c, *ptr.Get(t))
				assert.True(t, ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					assert.Must(t).NoError(err)
					assert.True(t, found, `entity was expected to be stored`)
					assert.Must(t).Equal(*ptr.Get(t), got)
				})
			})
		})

		s.When(`entity is a newer version compared to the stored one`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) *ENT {
				p := ptr.Super(t)
				assert.NoError(t, subject.Save(c.MakeContext(t), p))
				if bif, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
					c.Helper().IsPresent(t, bif, c.MakeContext(t), c.IDA.Get(*p))
				}

				c.ModifyEntity(t, p) // change entity to represent an update state
				return p
			}).EagerLoading(s)

			s.Then(`it will be updated with the new version`, func(t *testcase.T) {
				assert.Must(t).NoError(act(t))

				entID, ok := lookupNonZeroID[ID](c, *ptr.Get(t))
				assert.True(t, ok, `entity should have id`)

				t.Eventually(func(t *testcase.T) {
					got, found, err := shouldFindByID(t, c, subject, ctx.Get(t), entID)
					assert.Must(t).NoError(err)
					assert.True(t, found, `entity was expected to be stored`)
					assert.Must(t).Equal(*ptr.Get(t), got)
				})
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				allFinder, ok := subject.(crud.AllFinder[ENT])
				if !ok {
					t.Skipf("unable to continue with the test, crud.AllFinder is not implemented in %T", subject)
				}

				var initialCount int
				for {
					var counts = map[int]struct{}{}
					t.Random.Repeat(5, 7, func() {
						c, err := iterkit.CountE(allFinder.FindAll(ctx.Get(t)))
						assert.NoError(t, err)
						counts[c] = struct{}{}
						runtime.Gosched()
					})
					if len(counts) == 1 {
						for c := range counts {
							initialCount = c
						}
						break
					}
				}

				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					vs, err := iterkit.CollectE(allFinder.FindAll(ctx.Get(t)))
					assert.NoError(t, err)

					var count = len(vs)
					assert.Equal(t, initialCount, count)
				})
			})
		})

		if c.OnePhaseCommit != nil {
			s.Context("OnePhaseCommitProtocol", func(s *testcase.Spec) {
				s.Test(`BeginTx -> Save -> CommitTx will create the entity in the resource`, func(t *testcase.T) {
					tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
					assert.Must(t).NoError(err)

					ptr := pointer.Of(c.MakeEntity(t))
					assert.NoError(t, subject.Save(tx, ptr))
					id := c.Helper().HasID(t, ptr)

					if ByIDFinderOK {
						t.Eventually(func(t *testcase.T) {
							got, found, err := byIDF.FindByID(tx, id)
							assert.NoError(t, err)
							assert.True(t, found)
							assert.Equal(t, *ptr, got)
						})
						{
							_, found, err := byIDF.FindByID(c.MakeContext(t), id)
							assert.NoError(t, err)
							assert.False(t, found)
						}
					}

					assert.NoError(t, c.OnePhaseCommit.CommitTx(tx))
					t.Cleanup(func() { tryDelete(t, c, subject, *ptr) })

					if ByIDFinderOK {
						t.Eventually(func(t *testcase.T) {
							got, found, err := byIDF.FindByID(c.MakeContext(t), id)
							assert.NoError(t, err)
							assert.True(t, found)
							assert.Equal(t, *ptr, got)
						})
					}
				})

				s.Test(`BeginTx -> Save -> Rollback will undo the entity creation in the resource`, func(t *testcase.T) {
					tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
					assert.Must(t).NoError(err)

					ptr := pointer.Of(c.MakeEntity(t))
					assert.NoError(t, subject.Save(tx, ptr))
					id := c.Helper().HasID(t, ptr)

					if ByIDFinderOK {
						t.Eventually(func(t *testcase.T) {
							got, found, err := byIDF.FindByID(tx, id)
							assert.NoError(t, err)
							assert.True(t, found)
							assert.Equal(t, *ptr, got)
						})
						{
							_, found, err := byIDF.FindByID(c.MakeContext(t), id)
							assert.NoError(t, err)
							assert.False(t, found)
						}
					}

					assert.NoError(t, c.OnePhaseCommit.RollbackTx(tx))
					t.Cleanup(func() { tryDelete(t, c, subject, *ptr) })

					if ByIDFinderOK {
						_, found, err := byIDF.FindByID(c.MakeContext(t), id)
						assert.NoError(t, err)
						assert.False(t, found)
					}
				})

				s.Test(`A finished transaction will make Save yield error`, func(t *testcase.T) {
					tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
					assert.Must(t).NoError(err)

					assert.NoError(t, random.Pick(t.Random, c.OnePhaseCommit.CommitTx, c.OnePhaseCommit.RollbackTx)(tx))

					ptr := pointer.Of(c.MakeEntity(t))
					assert.Error(t, subject.Save(tx, ptr))
				})
			})
		}
	})

	return s.AsSuite("crud.Save")
}
