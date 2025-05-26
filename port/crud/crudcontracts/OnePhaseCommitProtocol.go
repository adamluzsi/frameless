package crudcontracts

import (
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/port/comproto/comprotocontracts"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/spechelper"

	"go.llib.dev/testcase"
)

func OnePhaseCommitProtocol[ENT, ID any](subject any, manager comproto.OnePhaseCommitProtocol, opts ...Option[ENT, ID]) contract.Contract {
	c := option.Use(opts)
	s := testcase.NewSpec(nil)
	s.HasSideEffect()

	s.BeforeAll(func(tb testing.TB) {
		spechelper.TryCleanup(tb, c.MakeContext(tb), subject)
	})

	var validSubject bool
	s.AfterAll(func(tb testing.TB) {
		if validSubject {
			return
		}
		if testingRunFlagProvided() {
			return
		}
		tb.Logf("OnePhaseCommitProtocol contract's test subject (%T) doesn't implement any testable method.", subject)
		tb.Log("Could it be that you passed the wrong testing subject as argument to the contract?")
	})

	s.Context("implements basic commit protocol contract",
		comprotocontracts.OnePhaseCommitProtocol(manager, &comprotocontracts.Config{
			MakeContext: c.MakeContext,
		}).Spec)

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {
		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := manager.BeginTx(ctx)
			t.Must.NoError(err)
			t.Must.NoError(manager.CommitTx(ctx))
			t.Must.NotNil(manager.CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := manager.BeginTx(ctx)
			t.Must.NoError(err)
			t.Must.NoError(manager.RollbackTx(ctx))
			t.Must.NotNil(manager.RollbackTx(ctx))
		})

		if subject, ok := subject.(crd[ENT, ID]); ok {
			validSubject = true
			specOPCPCRD(s, subject, manager, opts...)
		}

		if subject, ok := subject.(subjectSpecOPCPPurger[ENT, ID]); ok {
			validSubject = true
			s.Describe(`.Purger`, func(s *testcase.Spec) {
				specOPCPPurger[ENT, ID](s, subject, manager, opts...)
			})
		}

		if subject, ok := subject.(crud.Saver[ENT]); ok {
			validSubject = true
			s.Describe(`.Saver`, func(s *testcase.Spec) {
				specOPCPSaver[ENT, ID](s, subject, manager, opts...)
			})
		}
	})

	return s.AsSuite("OnePhaseCommitProtocol")
}

func specOPCPCRD[ENT, ID any](s *testcase.Spec, subject crd[ENT, ID], manager comproto.OnePhaseCommitProtocol, opts ...Option[ENT, ID]) {
	c := option.Use[Config[ENT, ID]](opts)

	s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)
		ptr := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, tx, ptr)
		id := c.Helper().HasID(t, ptr)

		t.Must.NoError(manager.CommitTx(tx))
		t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

		t.Log(`using the tx context after commit should yield error`)
		_, _, err = subject.FindByID(tx, id)
		t.Must.NotNil(err)
		t.Must.NotNil(subject.Create(tx, pointer.Of(c.MakeEntity(t))))

		if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
			shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
				return allFinder.FindAll(tx)
			})
		}

		if updater, ok := subject.(crud.Updater[ENT]); ok {
			t.Must.NotNil(updater.Update(tx, ptr),
				assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
					subject)))
		}

		t.Must.NotNil(subject.DeleteByID(tx, id))
		if allDeleter, ok := subject.(crud.AllDeleter); ok {
			t.Must.NotNil(allDeleter.DeleteAll(tx))
		}

		// crudtest.Waiter.Wait()
	})

	s.Test(`BeginTx+RollbackTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
		ctx := c.MakeContext(t)
		ctx, err := manager.BeginTx(ctx)
		t.Must.NoError(err)
		p := pointer.Of(c.MakeEntity(t))
		t.Must.NoError(subject.Create(ctx, p))
		id, _ := lookupID[ID](c, *p)
		t.Must.NoError(manager.RollbackTx(ctx))

		_, _, err = subject.FindByID(ctx, id)
		t.Must.NotNil(err)

		if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
			shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
				return allFinder.FindAll(ctx)
			})
		}

		t.Must.NotNil(subject.Create(ctx, pointer.Of(c.MakeEntity(t))))

		if updater, ok := subject.(crud.Updater[ENT]); ok {
			t.Must.NotNil(updater.Update(ctx, p),
				assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
					subject)))
		}

		t.Must.NotNil(subject.DeleteByID(ctx, id))

		if allDeleter, ok := subject.(crud.AllDeleter); ok {
			t.Must.NotNil(allDeleter.DeleteAll(ctx))
		}
	})

	s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)

		entity := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, tx, entity)
		id := c.Helper().HasID(t, entity)
		t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

		c.Helper().IsPresent(t, subject, tx, id)              // can be found in tx Context
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id) // is absent from the global Context

		t.Must.NoError(manager.CommitTx(tx)) // after the commit

		actually := c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
		t.Must.Equal(entity, actually)
	})

	s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)
		entity := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, tx, entity)

		id := c.Helper().HasID(t, entity)
		c.Helper().IsPresent(t, subject, tx, id)
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)

		t.Must.NoError(manager.RollbackTx(tx))

		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
	})

	s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
		ctx := c.MakeContext(t)
		entity := pointer.Of(c.MakeEntity(t))

		c.Helper().Create(t, subject, ctx, entity)
		id := c.Helper().HasID(t, entity)
		t.Defer(subject.DeleteByID, ctx, id)

		tx, err := manager.BeginTx(ctx)
		t.Must.NoError(err)

		c.Helper().IsPresent(t, subject, tx, id)
		t.Must.NoError(subject.DeleteByID(tx, id))
		c.Helper().IsAbsent(t, subject, tx, id)

		// in global Context it is findable
		c.Helper().IsPresent(t, subject, c.MakeContext(t), id)

		t.Must.NoError(manager.CommitTx(tx))
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
	})

	s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
		ctx := c.MakeContext(t)
		entity := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, ctx, entity)
		id := c.Helper().HasID(t, entity)

		tx, err := manager.BeginTx(ctx)
		t.Must.NoError(err)
		c.Helper().IsPresent(t, subject, tx, id)
		t.Must.NoError(subject.DeleteByID(tx, id))
		c.Helper().IsAbsent(t, subject, tx, id)
		c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
		t.Must.NoError(manager.RollbackTx(tx))
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
		t.Must.NoError(err)
		t.Log(`given tx1 is began`)

		e1 := pointer.Of(c.MakeEntity(t))
		t.Must.NoError(subject.Create(tx1, e1))
		c.Helper().IsPresent(t, subject, tx1, c.Helper().HasID(t, e1))
		c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
		t.Logf("and e1 is created in tx1: %#v", e1)

		tx2InTx1, err := manager.BeginTx(tx1)
		t.Must.NoError(err)
		t.Log(`and tx2 is began using tx1 as a base`)

		e2 := pointer.Of(c.MakeEntity(t))
		t.Must.NoError(subject.Create(tx2InTx1, e2))
		c.Helper().IsPresent(t, subject, tx2InTx1, c.Helper().HasID(t, e2))     // tx2 can see e2
		c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2)) // global don't see e2
		t.Logf(`and e2 is created in tx2 %#v`, e2)

		t.Log(`before commit, entities should be absent from the resource`)
		c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
		c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2))

		t.Must.NoError(manager.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
		t.Must.NoError(manager.CommitTx(tx1), `"outer" comproto should be considered done`)

		t.Log(`after everything is committed, entities should be in the resource`)
		c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e1))
		c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e2))
	})
}

func specOPCPSaver[ENT, ID any](s *testcase.Spec, subject crud.Saver[ENT], manager comproto.OnePhaseCommitProtocol, opts ...Option[ENT, ID]) {
	c := option.Use[Config[ENT, ID]](opts)
	_, gotByIDFinder := subject.(crud.ByIDFinder[ENT, ID])
	_, gotByIDDeleter := subject.(crud.ByIDDeleter[ID])

	s.Test(`BeginTx+RollbackTx / Save`, func(t *testcase.T) {
		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)

		ent := c.MakeEntity(t)
		c.Helper().Save(t, subject, tx, &ent)
		c.Helper().HasID(t, &ent)
		t.Must.NoError(manager.RollbackTx(tx))
	})

	if gotByIDFinder {
		subject := subject.(interface {
			crud.Saver[ENT]
			crud.ByIDFinder[ENT, ID]
		})

		s.Test(`BeginTx+RollbackTx / Save+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			t.Must.NoError(err)
			ent := c.MakeEntity(t)
			c.Helper().Save(t, subject, tx, &ent)

			id := c.Helper().HasID(t, &ent)
			c.Helper().IsPresent(t, subject, tx, id)
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)

			t.Must.NoError(manager.RollbackTx(tx))

			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
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
			t.Must.NoError(err)
			t.Log(`given tx1 is began`)

			e1 := pointer.Of(c.MakeEntity(t))
			t.Must.NoError(subject.Save(tx1, e1))
			c.Helper().IsPresent(t, subject, tx1, c.Helper().HasID(t, e1))
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := manager.BeginTx(tx1)
			t.Must.NoError(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := pointer.Of(c.MakeEntity(t))
			t.Must.NoError(subject.Save(tx2InTx1, e2))
			c.Helper().IsPresent(t, subject, tx2InTx1, c.Helper().HasID(t, e2))     // tx2 can see e2
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2)) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e1))
			c.Helper().IsAbsent(t, subject, globalContext, c.Helper().HasID(t, e2))

			t.Must.NoError(manager.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.NoError(manager.CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e1))
			c.Helper().IsPresent(t, subject, globalContext, c.Helper().HasID(t, e2))
		})
	}

	if gotByIDFinder && gotByIDDeleter {
		subject := subject.(interface {
			crud.Saver[ENT]
			crud.ByIDFinder[ENT, ID]
			crud.ByIDDeleter[ID]
		})

		s.Test(`BeginTx+CommitTx, Saver/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			t.Must.NoError(err)
			ptr := pointer.Of(c.MakeEntity(t))
			c.Helper().Save(t, subject, tx, ptr)
			id := c.Helper().HasID(t, ptr)

			t.Must.NoError(manager.CommitTx(tx))
			t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = subject.FindByID(tx, id)
			t.Must.NotNil(err)
			t.Must.NotNil(subject.Save(tx, pointer.Of(c.MakeEntity(t))),
				"expecte that .Save will respect that the tx in the context is already committed")

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
					return allFinder.FindAll(tx)
				})
			}

			if updater, ok := subject.(crud.Updater[ENT]); ok {
				t.Must.NotNil(updater.Update(tx, ptr),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			t.Must.NotNil(subject.DeleteByID(tx, id))
			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(tx))
			}

			// crudtest.Waiter.Wait()
		})

		s.Test(`BeginTx+RollbackTx, Saver/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ctx, err := manager.BeginTx(ctx)
			t.Must.NoError(err)
			p := pointer.Of(c.MakeEntity(t))
			t.Must.NoError(subject.Save(ctx, p))
			id, _ := lookupID[ID](c, *p)
			t.Must.NoError(manager.RollbackTx(ctx))

			_, _, err = subject.FindByID(ctx, id)
			t.Must.NotNil(err)

			if allFinder, ok := subject.(crud.AllFinder[ENT]); ok {
				shouldIterEventuallyError(t, func() iter.Seq2[ENT, error] {
					return allFinder.FindAll(ctx)
				})
			}

			t.Must.NotNil(subject.Save(ctx, pointer.Of(c.MakeEntity(t))))

			if updater, ok := subject.(crud.Updater[ENT]); ok {
				t.Must.NotNil(updater.Update(ctx, p),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			t.Must.NotNil(subject.DeleteByID(ctx, id))

			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(ctx))
			}
		})

		s.Test(`BeginTx+CommitTx / Save+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext(t))
			t.Must.NoError(err)

			ent := c.MakeEntity(t)
			c.Helper().Save(t, subject, tx, &ent)
			id := c.Helper().HasID(t, &ent)
			t.Defer(subject.DeleteByID, c.MakeContext(t), id) // cleanup

			c.Helper().IsPresent(t, subject, tx, id)              // can be found in tx Context
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id) // is absent from the global Context

			t.Must.NoError(manager.CommitTx(tx)) // after the commit

			actually := c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
			assert.Equal(t, &ent, actually)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ent := c.MakeEntity(t)

			_, _ = ctx, ent
			c.Helper().Save(t, subject, ctx, &ent)
			id := c.Helper().HasID(t, &ent)
			t.Defer(subject.DeleteByID, ctx, id)

			tx, err := manager.BeginTx(ctx)
			t.Must.NoError(err)
			_ = tx

			c.Helper().IsPresent(t, subject, tx, id)
			t.Must.NoError(subject.DeleteByID(tx, id))
			c.Helper().IsAbsent(t, subject, tx, id)

			// in global Context it is findable
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)

			t.Must.NoError(manager.CommitTx(tx))
			c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext(t)
			ent := c.MakeEntity(t)
			c.Helper().Save(t, subject, ctx, &ent)
			id := c.Helper().HasID(t, &ent)

			tx, err := manager.BeginTx(ctx)
			t.Must.NoError(err)
			c.Helper().IsPresent(t, subject, tx, id)
			t.Must.NoError(subject.DeleteByID(tx, id))
			c.Helper().IsAbsent(t, subject, tx, id)
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
			t.Must.NoError(manager.RollbackTx(tx))
			c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
		})
	}
}

type subjectSpecOPCPPurger[ENT, ID any] interface {
	crd[ENT, ID]
	purgerSubjectResource[ENT, ID]
}

func specOPCPPurger[ENT, ID any](s *testcase.Spec, subject subjectSpecOPCPPurger[ENT, ID], manager comproto.OnePhaseCommitProtocol, opts ...Option[ENT, ID]) {
	c := option.Use[Config[ENT, ID]](opts)

	s.Test(`entity created prior to transaction won't be affected by a purge after a rollback`, func(t *testcase.T) {
		ptr := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, c.MakeContext(t), ptr)
		id := c.Helper().HasID(t, ptr)

		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)

		t.Must.NoError(subject.Purge(tx))
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
		c.Helper().IsPresent(t, subject, c.MakeContext(t), id)

		t.Must.NoError(manager.RollbackTx(tx))
		c.Helper().IsPresent(t, subject, c.MakeContext(t), id)
	})

	s.Test(`entity created prior to transaction will be removed by a purge after the commit`, func(t *testcase.T) {
		ptr := pointer.Of(c.MakeEntity(t))
		c.Helper().Create(t, subject, c.MakeContext(t), ptr)
		id := c.Helper().HasID(t, ptr)

		tx, err := manager.BeginTx(c.MakeContext(t))
		t.Must.NoError(err)

		t.Must.NoError(subject.Purge(tx))
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
		c.Helper().IsPresent(t, subject, c.MakeContext(t), id)

		t.Must.NoError(manager.CommitTx(tx))
		c.Helper().IsAbsent(t, subject, c.MakeContext(t), id)
	})
}
