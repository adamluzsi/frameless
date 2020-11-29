package specs

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type OnePhaseCommitProtocol struct {
	T              interface{}
	FixtureFactory FixtureFactory
	Subject        interface {
		minimumRequirements
		resources.OnePhaseCommitProtocol
	}
}

func (spec OnePhaseCommitProtocol) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec OnePhaseCommitProtocol) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec OnePhaseCommitProtocol) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	s.HasSideEffect()

	s.Around(func(t *testcase.T) func() {
		clean := func() {
			require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
		}
		clean()
		return clean
	})

	s.Context(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on context with finished tx`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			t.Defer(spec.Subject.DeleteByID, spec.Context(), spec.T, id)
			require.Nil(t, spec.Subject.CommitTx(ctx))

			_, err = spec.Subject.FindByID(ctx, spec.T, id)
			require.Error(t, err)
			require.Error(t, spec.Subject.Create(ctx, spec.FixtureFactory.Create(spec.T)))
			require.Error(t, spec.Subject.FindAll(ctx, spec.T).Err())

			if updater, ok := spec.Subject.(resources.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.Subject))
			}

			require.Error(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			require.Error(t, spec.Subject.DeleteAll(ctx, spec.T))
		})
		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on context with finished tx`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			require.Nil(t, spec.Subject.RollbackTx(ctx))

			_, err = spec.Subject.FindByID(ctx, spec.T, id)
			require.Error(t, err)
			require.Error(t, spec.Subject.FindAll(ctx, spec.T).Err())
			require.Error(t, spec.Subject.Create(ctx, spec.FixtureFactory.Create(spec.T)))

			if updater, ok := spec.Subject.(resources.Updater); ok {
				require.Error(t, updater.Update(ctx, ptr),
					fmt.Sprintf(`because %T implements resource.Updater it was expected to also yields error on update with finished tx`,
						spec.Subject))
			}

			require.Error(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			require.Error(t, spec.Subject.DeleteAll(ctx, spec.T))
		})

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id := HasID(t, entity)

			IsFindable(t, spec.Subject, ctx, newEntityFunc(spec.T), id)          // can be found in tx context
			IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id) // is absent from the global context

			require.Nil(t, spec.Subject.CommitTx(ctx)) // after the commit

			actually := IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
			require.Equal(t, entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))

			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)

			found, err := spec.Subject.FindByID(spec.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)

			require.Nil(t, spec.Subject.RollbackTx(ctx))

			found, err = spec.Subject.FindByID(spec.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id := HasID(t, entity)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)

			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			IsFindable(t, spec.Subject, ctx, newEntityFunc(spec.T), id)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			IsAbsent(t, spec.Subject, ctx, newEntityFunc(spec.T), id)

			// in global context it is findable
			IsFindable(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)

			require.Nil(t, spec.Subject.CommitTx(ctx))
			IsAbsent(t, spec.Subject, spec.Context(), newEntityFunc(spec.T), id)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := spec.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			IsFindable(t, spec.Subject, ctx, spec.newEntity, id)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.T, id))
			IsAbsent(t, spec.Subject, ctx, spec.newEntity, id)
			IsFindable(t, spec.Subject, spec.Context(), spec.newEntity, id)
			require.Nil(t, spec.Subject.RollbackTx(ctx))
			IsFindable(t, spec.Subject, spec.Context(), spec.newEntity, id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.CommitTx(ctx))
			require.Error(t, spec.Subject.CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.RollbackTx(ctx))
			require.Error(t, spec.Subject.RollbackTx(ctx))
		})

		s.Test(`BeginTx should be callable multiple times to ensure  emulate multi level transaction`, func(t *testcase.T) {
			t.Log(
				`Even if the current driver or resource don't support multi level transactions`,
				`It should still accept multiple transaction begin for a given context`,
				`The benefit of this is that low level components that needs to ensure transactional execution,`,
				`they should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor itself.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			defer spec.Subject.DeleteAll(spec.Context(), spec.T)

			var globalContext = spec.Context()

			tx1, err := spec.Subject.BeginTx(globalContext)
			require.Nil(t, err)
			e1 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(tx1, e1))
			IsFindable(t, spec.Subject, tx1, newEntityFunc(spec.T), HasID(t, e1))

			tx2InTx1, err := spec.Subject.BeginTx(globalContext)
			require.Nil(t, err)

			e2 := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(tx2InTx1, e2))
			IsFindable(t, spec.Subject, tx2InTx1, newEntityFunc(spec.T), HasID(t, e2)) // tx2 entity should be visible
			IsFindable(t, spec.Subject, tx1, newEntityFunc(spec.T), HasID(t, e1))      // so the entity made in tx1

			t.Log(`before commit, entities should be absent from the resource`)
			IsAbsent(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e1))
			IsAbsent(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e2))

			require.Nil(t, spec.Subject.CommitTx(tx2InTx1), `"inner" tx should be considered done`)
			require.Nil(t, spec.Subject.CommitTx(tx1), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			IsFindable(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e1))
			IsFindable(t, spec.Subject, globalContext, newEntityFunc(spec.T), HasID(t, e2))
		})
	})
}

func (spec OnePhaseCommitProtocol) newEntity() interface{} {
	return reflect.New(reflect.TypeOf(spec.T)).Interface()
}

func (spec OnePhaseCommitProtocol) Context() context.Context {
	return spec.FixtureFactory.Context()
}
