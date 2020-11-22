package specs

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
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
			require.Nil(t, spec.Subject.DeleteAll(spec.FixtureFactory.Context(), spec.T))
		}
		clean()
		return clean
	})

	s.Context(`OnePhaseCommitProtocol`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx -> Creator/Reader/Deleter methods yields error on context with finished tx`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ptr := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, ptr))
			id, _ := resources.LookupID(ptr)
			t.Defer(spec.Subject.DeleteByID, spec.FixtureFactory.Context(), spec.T, id)
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
			ctx := spec.FixtureFactory.Context()
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
			ctx := spec.FixtureFactory.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))

			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)

			found, err := spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)

			require.Nil(t, spec.Subject.CommitTx(ctx))

			actually := spec.newEntity()
			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), actually, id)
			require.Nil(t, err)
			require.True(t, found)
			require.Equal(t, entity, actually)
		})

		s.Test(`BeginTx+RollbackTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))

			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)

			found, err := spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)

			require.Nil(t, spec.Subject.RollbackTx(ctx))

			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)
		})

		s.Test(`BeginTx+CommitTx / committed delete during transaction`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			entity := spec.FixtureFactory.Create(spec.T)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.T, id)

			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			found, err := spec.Subject.FindByID(ctx, spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.T, id))

			found, err = spec.Subject.FindByID(ctx, spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)

			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)

			require.Nil(t, spec.Subject.CommitTx(ctx))

			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)
		})

		s.Test(`BeginTx+RollbackTx / reverted delete during transaction`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
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
			IsFindable(t, spec.Subject, spec.FixtureFactory.Context(), spec.newEntity, id)
			require.Nil(t, spec.Subject.RollbackTx(ctx))
			IsFindable(t, spec.Subject, spec.FixtureFactory.Context(), spec.newEntity, id)
		})

		s.Test(`CommitTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.CommitTx(ctx))
			require.Error(t, spec.Subject.CommitTx(ctx))
		})

		s.Test(`RollbackTx multiple times will yield error`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
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

			defer spec.Subject.DeleteAll(spec.FixtureFactory.Context(), spec.T)

			var (
				ctx   = spec.FixtureFactory.Context()
				count int
				err   error
			)

			ctxWithLevel1Tx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.Create(ctxWithLevel1Tx, spec.FixtureFactory.Create(spec.T)))
			count, err = iterators.Count(spec.Subject.FindAll(ctxWithLevel1Tx, spec.T))
			require.Nil(t, err)
			require.Equal(t, 1, count)

			ctxWithLevel2Tx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			require.Nil(t, err)
			require.Nil(t, spec.Subject.Create(ctxWithLevel1Tx, spec.FixtureFactory.Create(spec.T)))
			count, err = iterators.Count(spec.Subject.FindAll(ctxWithLevel1Tx, spec.T))
			require.Nil(tb, err)
			require.Equal(tb, 2, count)

			t.Log(`before commit, entities should be absent from the resource`)
			count, err = iterators.Count(spec.Subject.FindAll(ctx, spec.T))
			require.Nil(t, err)
			require.Equal(t, 0, count)

			require.Nil(t, spec.Subject.CommitTx(ctxWithLevel2Tx), `"inner" tx should be considered done`)
			require.Nil(t, spec.Subject.CommitTx(ctxWithLevel1Tx), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, entities should be in the resource`)
			AsyncTester.Assert(t, func(tb testing.TB) {
				count, err = iterators.Count(spec.Subject.FindAll(ctx, spec.T))
				require.Nil(tb, err)
				require.Equal(tb, 2, count)
			})
		})
	})
}


func (spec OnePhaseCommitProtocol) newEntity() interface{} {
	return reflect.New(reflect.TypeOf(spec.T)).Interface()
}
