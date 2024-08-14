package crudcontracts

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/port/comproto/comprotocontracts"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/port/comproto"
	crudtest "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/spechelper"

	"go.llib.dev/testcase"
)

func OnePhaseCommitProtocol[Entity, ID any](subject crd[Entity, ID], manager comproto.OnePhaseCommitProtocol, opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)
	s.HasSideEffect()

	s.BeforeAll(func(tb testing.TB) {
		spechelper.TryCleanup(tb, c.MakeContext(), subject)
	})

	s.Context("implements basic commit protocol contract",
		comprotocontracts.OnePhaseCommitProtocol(manager, &comprotocontracts.Config{
			MakeContext: c.MakeContext,
		}).Spec)

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			ptr := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, subject, tx, ptr)
			id := crudtest.HasID[Entity, ID](t, pointer.Deref(ptr))

			t.Must.Nil(manager.CommitTx(tx))
			t.Defer(subject.DeleteByID, c.MakeContext(), id) // cleanup

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = subject.FindByID(tx, id)
			t.Must.NotNil(err)
			t.Must.NotNil(subject.Create(tx, pointer.Of(c.MakeEntity(t))))

			if allFinder, ok := subject.(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(tx).Err())
			}

			if updater, ok := subject.(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(tx, ptr),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			t.Must.NotNil(subject.DeleteByID(tx, id))
			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(tx))
			}

			crudtest.Waiter.Wait()
		})

		s.Test(`BeginTx+RollbackTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.MakeContext()
			ctx, err := manager.BeginTx(ctx)
			t.Must.Nil(err)
			ptr := pointer.Of(c.MakeEntity(t))
			t.Must.NoError(subject.Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Must.NoError(manager.RollbackTx(ctx))

			_, _, err = subject.FindByID(ctx, id)
			t.Must.NotNil(err)

			if allFinder, ok := subject.(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(ctx).Err())
			}

			t.Must.NotNil(subject.Create(ctx, pointer.Of(c.MakeEntity(t))))

			if updater, ok := subject.(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(ctx, ptr),
					assert.Message(fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						subject)))
			}

			t.Must.NotNil(subject.DeleteByID(ctx, id))

			if allDeleter, ok := subject.(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(ctx))
			}
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext())
			t.Must.Nil(err)

			entity := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, subject, tx, entity)
			id := crudtest.HasID[Entity, ID](t, pointer.Deref(entity))
			t.Defer(subject.DeleteByID, c.MakeContext(), id) // cleanup

			crudtest.IsPresent[Entity, ID](t, subject, tx, id)             // can be found in tx Context
			crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), id) // is absent from the global Context

			t.Must.Nil(manager.CommitTx(tx)) // after the commit

			actually := crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), id)
			t.Must.Equal(entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := manager.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			entity := pointer.Of(c.MakeEntity(t))
			//t.Must.Nil( Spesubject.Create(tx, entity))
			crudtest.Create[Entity, ID](t, subject, tx, entity)

			id := crudtest.HasID[Entity, ID](t, pointer.Deref(entity))
			crudtest.IsPresent[Entity, ID](t, subject, tx, id)
			crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), id)

			t.Must.Nil(manager.RollbackTx(tx))

			crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext()
			entity := pointer.Of(c.MakeEntity(t))

			crudtest.Create[Entity, ID](t, subject, ctx, entity)
			id := crudtest.HasID[Entity, ID](t, pointer.Deref(entity))
			t.Defer(subject.DeleteByID, ctx, id)

			tx, err := manager.BeginTx(ctx)
			t.Must.Nil(err)

			crudtest.IsPresent[Entity, ID](t, subject, tx, id)
			t.Must.Nil(subject.DeleteByID(tx, id))
			crudtest.IsAbsent[Entity, ID](t, subject, tx, id)

			// in global Context it is findable
			crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), id)

			t.Must.Nil(manager.CommitTx(tx))
			crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.MakeContext()
			entity := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, subject, ctx, entity)
			id := crudtest.HasID[Entity, ID](t, pointer.Deref(entity))

			tx, err := manager.BeginTx(ctx)
			t.Must.Nil(err)
			crudtest.IsPresent[Entity, ID](t, subject, tx, id)
			t.Must.Nil(subject.DeleteByID(tx, id))
			crudtest.IsAbsent[Entity, ID](t, subject, tx, id)
			crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), id)
			t.Must.Nil(manager.RollbackTx(tx))
			crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext()
			ctx, err := manager.BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(manager.CommitTx(ctx))
			t.Must.NotNil(manager.CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.MakeContext()
			ctx, err := manager.BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(manager.RollbackTx(ctx))
			t.Must.NotNil(manager.RollbackTx(ctx))
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

			t.Defer(crudtest.DeleteAll[Entity, ID], t, subject, c.MakeContext())

			var globalContext = c.MakeContext()

			tx1, err := manager.BeginTx(globalContext)
			t.Must.Nil(err)
			t.Log(`given tx1 is began`)

			e1 := pointer.Of(c.MakeEntity(t))
			t.Must.Nil(subject.Create(tx1, e1))
			crudtest.IsPresent[Entity, ID](t, subject, tx1, crudtest.HasID[Entity, ID](t, pointer.Deref(e1)))
			crudtest.IsAbsent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e1)))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := manager.BeginTx(tx1)
			t.Must.Nil(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := pointer.Of(c.MakeEntity(t))
			t.Must.Nil(subject.Create(tx2InTx1, e2))
			crudtest.IsPresent[Entity, ID](t, subject, tx2InTx1, crudtest.HasID[Entity, ID](t, pointer.Deref(e2)))     // tx2 can see e2
			crudtest.IsAbsent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e2))) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			crudtest.IsAbsent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e1)))
			crudtest.IsAbsent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e2)))

			t.Must.Nil(manager.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.Nil(manager.CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			crudtest.IsPresent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e1)))
			crudtest.IsPresent[Entity, ID](t, subject, globalContext, crudtest.HasID[Entity, ID](t, pointer.Deref(e2)))
		})

		if subject, ok := any(subject).(subjectSpecOPCPPurger[Entity, ID]); ok {
			s.Describe(`.Purger`, func(s *testcase.Spec) {
				specOPCPPurger[Entity, ID](s, subject, manager, opts...)
			})
		}
	})

	return s.AsSuite("OnePhaseCommitProtocol")
}

type subjectSpecOPCPPurger[Entity, ID any] interface {
	crd[Entity, ID]
	purgerSubjectResource[Entity, ID]
}

func specOPCPPurger[Entity, ID any](s *testcase.Spec, subject subjectSpecOPCPPurger[Entity, ID], manager comproto.OnePhaseCommitProtocol, opts ...Option[Entity, ID]) {
	c := option.Use[Config[Entity, ID]](opts)

	s.Test(`entity created prior to transaction won't be affected by a purge after a rollback`, func(t *testcase.T) {
		ptr := pointer.Of(c.MakeEntity(t))
		crudtest.Create[Entity, ID](t, subject, c.MakeContext(), ptr)

		tx, err := manager.BeginTx(c.MakeContext())
		t.Must.Nil(err)

		t.Must.Nil(subject.Purge(tx))
		crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))
		crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))

		t.Must.Nil(manager.RollbackTx(tx))
		crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))
	})

	s.Test(`entity created prior to transaction will be removed by a purge after the commit`, func(t *testcase.T) {
		ptr := pointer.Of(c.MakeEntity(t))
		crudtest.Create[Entity, ID](t, subject, c.MakeContext(), ptr)

		tx, err := manager.BeginTx(c.MakeContext())
		t.Must.Nil(err)

		t.Must.Nil(subject.Purge(tx))
		crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))
		crudtest.IsPresent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))

		t.Must.Nil(manager.CommitTx(tx))
		crudtest.IsAbsent[Entity, ID](t, subject, c.MakeContext(), crudtest.HasID[Entity, ID](t, pointer.Deref(ptr)))
	})
}
