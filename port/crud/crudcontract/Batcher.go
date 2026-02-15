package crudcontract

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func Batcher[ENT, ID any, Batch crud.Batch[ENT]](subject crud.Batcher[ENT, Batch], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)

	crt, CreatorOK := subject.(crud.Creator[ENT])
	byIDD, byIDDeleterOK := subject.(crud.ByIDDeleter[ID])
	byIDF, ByIDFinderOK := subject.(crud.ByIDFinder[ENT, ID])
	allF, AllFinderOK := subject.(crud.AllFinder[ENT])

	var (
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		values = testcase.Let(s, func(t *testcase.T) []ENT {
			return random.Slice(t.Random.IntBetween(3, 7), func() ENT {
				return c.MakeEntity(t)
			})
		})
	)
	act := func(t *testcase.T) error {
		var do = func() error {
			ctx := ctxVar.Get(t)
			batch := subject.Batch(ctx)
			for _, v := range values.Get(t) {
				if err := batch.Add(v); err != nil {
					return err
				}
			}
			return batch.Close()
		}
		var err = do()
		if err == nil {
			t.Defer(spechelper.TryCleanup, t, c.MakeContext(t), subject)
		}
		return err
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), subject)
	})

	s.Then("it should succeed with the batch operation", func(t *testcase.T) {
		assert.NoError(t, act(t))
	})

	if AllFinderOK {
		s.Then(`added entities can be retrieved with #FindAll`, func(t *testcase.T) {
			assert.NoError(t, act(t))

			t.Eventually(func(t *testcase.T) {
				gotVS, err := iterkit.CollectE(allF.FindAll(c.MakeContext(t)))
				assert.NoError(t, err)

				for _, exp := range values.Get(t) {
					assert.OneOf(t, gotVS, func(t testing.TB, got ENT) {
						if id, ok := c.IDA.Lookup(exp); ok {
							if zerokit.IsZero(id) {
								// nuke v's ID field to zero to make comparison easier
								var zeroID ID
								c.IDA.Set(&got, zeroID)
							} else {
								assert.Equal(t, c.IDA.Get(exp), id)
							}
						}
						assert.Equal(t, exp, got)
					})
				}
			})
		})
	}

	s.When(`ctx arg has an error`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context's error`, func(t *testcase.T) {
			assert.Must(t).ErrorIs(ctxVar.Get(t).Err(), act(t))
		})
	})

	s.Context(`ctx arg has an error eventually`, func(s *testcase.Spec) {
		values.Let(s, func(t *testcase.T) []ENT {
			// non zero slice list
			return random.Slice(t.Random.IntBetween(3, 7), func() ENT {
				return c.MakeEntity(t)
			})
		})

		s.Test("during Batch#Add", func(t *testcase.T) {
			ctx, cancel := context.WithCancel(c.MakeContext(t))

			last, ok := slicekit.Last(values.Get(t))
			assert.True(t, ok)

			batch := subject.Batch(ctx)
			defer batch.Close()

			for i := 0; i < len(values.Get(t))-1; i++ {
				assert.NoError(t, batch.Add(values.Get(t)[i]))
			}

			cancel()

			assert.Error(t, batch.Add(last))
			assert.Error(t, batch.Close())
		})

		s.Test("during Batch#Close", func(t *testcase.T) {
			ctx, cancel := context.WithCancel(c.MakeContext(t))

			var batch = subject.Batch(ctx)
			defer batch.Close()

			for _, v := range values.Get(t) {
				assert.NoError(t, batch.Add(v))
			}

			cancel()

			assert.Error(t, batch.Close())
		})
	})

	if (c.SupportIDReuse || c.SupportRecreate) && byIDDeleterOK {
		s.When(`entity ID is provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				for i, value := range values.Get(t) {
					if _, hasID := lookupID(c, value); hasID {
						continue
					}

					if !byIDDeleterOK {
						t.Skipf("unable to finish test as MakeEntity doesn't supply ID, and %T doesn't implement crud.ByIDDeleter", subject)
					}

					if !CreatorOK {
						t.Skipf("unable to finish test as MakeEntity doesn't supply ID, and %T doesn't implement crud.Creator", subject)
					}

					assert.NoError(t, crt.Create(c.MakeContext(t), &value))
					values.Get(t)[i] = value

					id := c.IDA.Get(value)
					if ByIDFinderOK {
						c.Helper().IsPresent(t, byIDF, c.MakeContext(t), id)
					}

					assert.Must(t).NoError(byIDD.DeleteByID(c.MakeContext(t), id))

					if ByIDFinderOK {
						c.Helper().IsAbsent(t, byIDF, c.MakeContext(t), id)
					}
				}
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				assert.Must(t).NoError(act(t))
			})

			if ByIDFinderOK {
				s.Then(`the persisted objects can be found with #FindByID`, func(t *testcase.T) {
					assert.Must(t).NoError(act(t))

					for _, value := range values.Get(t) {
						c.Helper().IsPresent(t, byIDF, c.MakeContext(t), c.IDA.Get(value))
					}
				})
			}

			if AllFinderOK {
				s.Then(`the persisted objects are retrieved with #FindAll`, func(t *testcase.T) {
					assert.Must(t).NoError(act(t))

					gotVS, err := iterkit.CollectE(allF.FindAll(c.MakeContext(t)))
					assert.NoError(t, err)

					for _, value := range values.Get(t) {
						id := c.IDA.Get(value)
						assert.OneOf(t, gotVS, func(t testing.TB, v ENT) {
							assert.Equal(t, id, c.IDA.Get(v))
						})
					}
				})
			}
		})
	}

	if c.OnePhaseCommit != nil {
		manager := c.OnePhaseCommit
		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			ptr := pointer.Of(c.MakeEntity(t))

			c.Helper().Create(t, subject, tx, ptr)
			id := c.Helper().HasID(t, ptr)

			assert.Must(t).NoError(manager.CommitTx(tx))
			t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = subject.FindByID(tx, id)
			assert.Must(t).NotNil(err)
			assert.Must(t).NotNil(subject.Create(tx, pointer.Of(c.MakeEntity(t))))

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
					return allFinder.FindAll(tx)
				})
			}

			if updater, ok := subject.(crud.Updater[ENT]); ok {
				assert.Must(t).NotNil(updater.Update(tx, ptr),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			assert.Must(t).NotNil(subject.DeleteByID(tx, id))
			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				assert.Must(t).NotNil(allDeleter.DeleteAll(tx))
			}

			// crudtest.Waiter.Wait()
		})

		s.Test(`BeginTx+RollbackTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := manager.BeginTx(ctx)
			assert.Must(t).NoError(err)
			p := pointer.Of(c.MakeEntity(t))
			assert.Must(t).NoError(subject.Create(ctx, p))
			id, _ := lookupID[ID](c, *p)
			assert.Must(t).NoError(manager.RollbackTx(ctx))

			_, _, err = subject.FindByID(ctx, id)
			assert.Must(t).NotNil(err)

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
					return allFinder.FindAll(ctx)
				})
			}

			assert.Must(t).NotNil(subject.Create(ctx, pointer.Of(c.MakeEntity(t))))

			if updater, ok := subject.(crud.Updater[ENT]); ok {
				assert.Must(t).NotNil(updater.Update(ctx, p),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			assert.Must(t).NotNil(subject.DeleteByID(ctx, id))

			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				assert.Must(t).NotNil(allDeleter.DeleteAll(ctx))
			}
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)

			entity := pointer.Of(c.MakeEntity(t))
			c.Helper().Create(t, subject, tx, entity)
			id := c.Helper().HasID(t, entity)
			t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

			c.Helper().IsPresent(t, subject, tx, id)              // can be found in tx Context
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id) // is absent from the global Context

			assert.Must(t).NoError(manager.CommitTx(tx)) // after the commit

			actually := c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
			assert.Must(t).Equal(entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			entity := pointer.Of(c.MakeEntity(t))
			c.Helper().Create(t, subject, tx, entity)

			id := c.Helper().HasID(t, entity)
			c.Helper().IsPresent(t, subject, tx, id)
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)

			assert.Must(t).NoError(manager.RollbackTx(tx))

			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			entity := pointer.Of(c.MakeEntity(t))

			c.Helper().Create(t, subject, ctx, entity)
			id := c.Helper().HasID(t, entity)
			t.Defer(subject.DeleteByID, ctx, id)

			tx, err := manager.BeginTx(ctx)
			assert.Must(t).NoError(err)

			c.Helper().IsPresent(t, subject, tx, id)
			assert.Must(t).NoError(subject.DeleteByID(tx, id))
			c.Helper().IsAbsent(t, subject, tx, id)

			// in global Context it is findable
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)

			assert.Must(t).NoError(manager.CommitTx(tx))
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			entity := pointer.Of(c.MakeEntity(t))
			c.Helper().Create(t, subject, ctx, entity)
			id := c.Helper().HasID(t, entity)

			tx, err := manager.BeginTx(ctx)
			assert.Must(t).NoError(err)
			c.Helper().IsPresent(t, subject, tx, id)
			assert.Must(t).NoError(subject.DeleteByID(tx, id))
			c.Helper().IsAbsent(t, subject, tx, id)
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
			assert.Must(t).NoError(manager.RollbackTx(tx))
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
		})

		s.Test(`BeginTx should be callable multiple times to ensure emulate multi level transaction`, func(t *testcase.T) {
			t.Log(
				`Even if the current driver or resource don't support multi level transactions`,
				`It should still accept multiple transaction begin for a given Context`,
				`The benefit of this is that low level components that needs to ensure transactional execution,`,
				`they should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor itself.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			t.Defer(crudtest.DeleteAll[ENT, ID], t, subject, c.MakeContext(t))

			var globalContext = c.MakeContext(t)

			tx1, err := manager.BeginTx(globalContext)
			assert.Must(t).NoError(err)
			t.Log(`given tx1 is began`)

			e1 := pointer.Of(c.MakeEntity(t))
			assert.Must(t).NoError(subject.Create(tx1, e1))
			c.Helper().IsPresent(t, subject, tx1, c.Helper().HasID(t, e1))
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := manager.BeginTx(tx1)
			assert.Must(t).NoError(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := pointer.Of(c.MakeEntity(t))
			assert.Must(t).NoError(subject.Create(tx2InTx1, e2))
			c.Helper().IsPresent(t, subject, tx2InTx1, c.Helper().HasID(t, e2))     // tx2 can see e2
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2)) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2))

			assert.Must(t).NoError(manager.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			assert.Must(t).NoError(manager.CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e1))
			c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e2))
		})
	}

	return s.AsSuite("Batcher")
}
