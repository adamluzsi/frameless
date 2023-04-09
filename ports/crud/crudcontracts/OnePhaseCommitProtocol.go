package crudcontracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/comproto/comprotocontracts"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/pkg/pointer"

	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/testcase"
)

type OnePhaseCommitProtocol[Entity, ID any] func(testing.TB) OnePhaseCommitProtocolSubject[Entity, ID]

type OnePhaseCommitProtocolSubject[Entity, ID any] struct {
	Resource      spechelper.CRD[Entity, ID]
	CommitManager comproto.OnePhaseCommitProtocol
	MakeContext   func() context.Context
	MakeEntity    func() Entity
}

func (c OnePhaseCommitProtocol[Entity, ID]) subject() testcase.Var[OnePhaseCommitProtocolSubject[Entity, ID]] {
	return testcase.Var[OnePhaseCommitProtocolSubject[Entity, ID]]{
		ID:   "OnePhaseCommitProtocolSubject",
		Init: func(t *testcase.T) OnePhaseCommitProtocolSubject[Entity, ID] { return c(t) },
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) manager() testcase.Var[comproto.OnePhaseCommitProtocol] {
	return testcase.Var[comproto.OnePhaseCommitProtocol]{
		ID: "commit protocol manager",
		Init: func(t *testcase.T) comproto.OnePhaseCommitProtocol {
			return c.subject().Get(t).CommitManager
		},
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) resource() testcase.Var[spechelper.CRD[Entity, ID]] {
	return testcase.Var[spechelper.CRD[Entity, ID]]{
		ID: "managed resource",
		Init: func(t *testcase.T) spechelper.CRD[Entity, ID] {
			return c.subject().Get(t).Resource
		},
	}
}

func (c OnePhaseCommitProtocol[Entity, ID]) Name() string {
	return "OnePhaseCommitProtocol"
}

func (c OnePhaseCommitProtocol[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c OnePhaseCommitProtocol[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c OnePhaseCommitProtocol[Entity, ID]) Spec(s *testcase.Spec) {
	s.HasSideEffect()

	// clean ahead before testing suite
	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() { spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(), c.resource().Get(t)) })
	})

	s.Context("implements basic commit protocol contract",
		comprotocontracts.OnePhaseCommitProtocol(func(tb testing.TB) comprotocontracts.OnePhaseCommitProtocolSubject {
			sub := c(tb)
			return comprotocontracts.OnePhaseCommitProtocolSubject{
				CommitManager: sub.CommitManager,
				MakeContext:   sub.MakeContext,
			}
		}).Spec)

	s.Describe(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {
		s.Test(`BeginTx+CommitTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.subject().Get(t).MakeContext())
			t.Must.Nil(err)
			ptr := pointer.Of(c.subject().Get(t).MakeEntity())
			Create[Entity, ID](t, c.resource().Get(t), tx, ptr)
			id := HasID[Entity, ID](t, pointer.Deref(ptr))

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))

			t.Log(`using the tx context after commit should yield error`)
			_, _, err = c.resource().Get(t).FindByID(tx, id)
			t.Must.NotNil(err)
			t.Must.NotNil(c.resource().Get(t).Create(tx, pointer.Of(c.subject().Get(t).MakeEntity())))

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(tx).Err())
			}

			if updater, ok := c.resource().Get(t).(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(tx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						c.resource().Get(t)))
			}

			t.Must.NotNil(c.resource().Get(t).DeleteByID(tx, id))
			if allDeleter, ok := c.resource().Get(t).(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(tx))
			}

			Waiter.Wait()
		})

		s.Test(`BeginTx+RollbackTx, Creator/Reader/Deleter methods yields error on Context with finished tx`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			ptr := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.NoError(c.resource().Get(t).Create(ctx, ptr))
			id, _ := extid.Lookup[ID](ptr)
			t.Must.NoError(c.manager().Get(t).RollbackTx(ctx))

			_, _, err = c.resource().Get(t).FindByID(ctx, id)
			t.Must.NotNil(err)

			if allFinder, ok := c.resource().Get(t).(crud.AllFinder[Entity]); ok {
				t.Must.NotNil(allFinder.FindAll(ctx).Err())
			}

			t.Must.NotNil(c.resource().Get(t).Create(ctx, pointer.Of(c.subject().Get(t).MakeEntity())))

			if updater, ok := c.resource().Get(t).(crud.Updater[Entity]); ok {
				t.Must.NotNil(updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished comproto`,
						c.resource().Get(t)))
			}

			t.Must.NotNil(c.resource().Get(t).DeleteByID(ctx, id))

			if allDeleter, ok := c.resource().Get(t).(crud.AllDeleter); ok {
				t.Must.NotNil(allDeleter.DeleteAll(ctx))
			}
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.subject().Get(t).MakeContext())
			t.Must.Nil(err)

			entity := pointer.Of(c.subject().Get(t).MakeEntity())
			Create[Entity, ID](t, c.resource().Get(t), tx, entity)
			id := HasID[Entity, ID](t, pointer.Deref(entity))

			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)                             // can be found in tx Context
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id) // is absent from the global Context

			t.Must.Nil(c.manager().Get(t).CommitTx(tx)) // after the commit

			actually := IsFindable[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)
			t.Must.Equal(entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			tx, err := c.manager().Get(t).BeginTx(c.subject().Get(t).MakeContext())
			t.Must.Nil(err)
			entity := pointer.Of(c.subject().Get(t).MakeEntity())
			//t.Must.Nil( Spec.resource().Get(t).Create(tx, entity))
			Create[Entity, ID](t, c.resource().Get(t), tx, entity)

			id := HasID[Entity, ID](t, pointer.Deref(entity))
			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)

			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))

			IsAbsent[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			entity := pointer.Of(c.subject().Get(t).MakeEntity())

			Create[Entity, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Entity, ID](t, pointer.Deref(entity))
			t.Defer(c.resource().Get(t).DeleteByID, ctx, id)

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)

			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Entity, ID](t, c.resource().Get(t), tx, id)

			// in global Context it is findable
			IsFindable[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)

			t.Must.Nil(c.manager().Get(t).CommitTx(tx))
			IsAbsent[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			entity := pointer.Of(c.subject().Get(t).MakeEntity())
			Create[Entity, ID](t, c.resource().Get(t), ctx, entity)
			id := HasID[Entity, ID](t, pointer.Deref(entity))

			tx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			IsFindable[Entity, ID](t, c.resource().Get(t), tx, id)
			t.Must.Nil(c.resource().Get(t).DeleteByID(tx, id))
			IsAbsent[Entity, ID](t, c.resource().Get(t), tx, id)
			IsFindable[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)
			t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
			IsFindable[Entity, ID](t, c.resource().Get(t), c.subject().Get(t).MakeContext(), id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(c.manager().Get(t).CommitTx(ctx))
			t.Must.NotNil(c.manager().Get(t).CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := c.subject().Get(t).MakeContext()
			ctx, err := c.manager().Get(t).BeginTx(ctx)
			t.Must.Nil(err)
			t.Must.Nil(c.manager().Get(t).RollbackTx(ctx))
			t.Must.NotNil(c.manager().Get(t).RollbackTx(ctx))
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

			t.Defer(DeleteAll[Entity, ID], t, c.resource().Get(t), c.subject().Get(t).MakeContext())

			var globalContext = c.subject().Get(t).MakeContext()

			tx1, err := c.manager().Get(t).BeginTx(globalContext)
			t.Must.Nil(err)
			t.Log(`given tx1 is began`)

			e1 := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.Nil(c.resource().Get(t).Create(tx1, e1))
			IsFindable[Entity, ID](t, c.resource().Get(t), tx1, HasID[Entity, ID](t, pointer.Deref(e1)))
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e1)))
			t.Logf("and e1 is created in tx1: %#v", e1)

			tx2InTx1, err := c.manager().Get(t).BeginTx(tx1)
			t.Must.Nil(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			e2 := pointer.Of(c.subject().Get(t).MakeEntity())
			t.Must.Nil(c.resource().Get(t).Create(tx2InTx1, e2))
			IsFindable[Entity, ID](t, c.resource().Get(t), tx2InTx1, HasID[Entity, ID](t, pointer.Deref(e2)))    // tx2 can see e2
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e2))) // global don't see e2
			t.Logf(`and e2 is created in tx2 %#v`, e2)

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e1)))
			IsAbsent[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e2)))

			t.Must.Nil(c.manager().Get(t).CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.Nil(c.manager().Get(t).CommitTx(tx1), `"outer" comproto should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e1)))
			IsFindable[Entity, ID](t, c.resource().Get(t), globalContext, HasID[Entity, ID](t, pointer.Deref(e2)))
		})

		s.Describe(`.Purger`, c.specPurger)
	})
}

func (c OnePhaseCommitProtocol[Entity, ID]) specPurger(s *testcase.Spec) {
	purger := func(t *testcase.T) purgerSubjectResource[Entity, ID] {
		p, ok := c.resource().Get(t).(purgerSubjectResource[Entity, ID])
		if !ok {
			t.Skipf(`%T doesn't supply contract.PurgerSubject`, c.resource().Get(t))
		}
		return p
	}

	s.Before(func(t *testcase.T) { purger(t) }) // guard clause

	s.Test(`entity created prior to transaction won't be affected by a purge after a rollback`, func(t *testcase.T) {
		ptr := pointer.Of(c.subject().Get(t).MakeEntity())
		Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))

		t.Must.Nil(c.manager().Get(t).RollbackTx(tx))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))
	})

	s.Test(`entity created prior to transaction will be removed by a purge after the commit`, func(t *testcase.T) {
		ptr := pointer.Of(c.subject().Get(t).MakeEntity())
		Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)

		tx, err := c.manager().Get(t).BeginTx(spechelper.ContextVar.Get(t))
		t.Must.Nil(err)

		t.Must.Nil(purger(t).Purge(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))
		IsFindable[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))

		t.Must.Nil(c.manager().Get(t).CommitTx(tx))
		IsAbsent[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), HasID[Entity, ID](t, pointer.Deref(ptr)))
	})
}
