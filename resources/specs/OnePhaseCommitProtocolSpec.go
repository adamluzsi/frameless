package specs

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources"
)

type OnePhaseCommitProtocolSpec struct {
	EntityType     interface{}
	FixtureFactory FixtureFactory
	Subject        interface {
		minimumRequirements
		resources.OnePhaseCommitProtocol
	}
}

func (spec OnePhaseCommitProtocolSpec) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec OnePhaseCommitProtocolSpec) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec OnePhaseCommitProtocolSpec) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	s.HasSideEffect()

	s.Around(func(t *testcase.T) func() {
		clean := func() {
			require.Nil(t, spec.Subject.DeleteAll(spec.FixtureFactory.Context(), spec.EntityType))
		}
		clean()
		return clean
	})

	s.Context(`OnePhaseCommitProtocolSpec`, func(s *testcase.Spec) {

		s.Test(`BeginTx+CommitTx / Create+FindByID`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Create(ctx, entity))

			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.EntityType, id)

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
			entity := spec.FixtureFactory.Create(spec.EntityType)
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
			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.EntityType, id)

			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			found, err := spec.Subject.FindByID(ctx, spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.EntityType, id))

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
			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, ctx, spec.EntityType, id)

			ctx, err := spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			found, err := spec.Subject.FindByID(ctx, spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)
			require.Nil(t, spec.Subject.DeleteByID(ctx, spec.EntityType, id))

			found, err = spec.Subject.FindByID(ctx, spec.newEntity(), id)
			require.Nil(t, err)
			require.False(t, found)

			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)

			require.Nil(t, spec.Subject.RollbackTx(ctx))

			found, err = spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)
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
				`it should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor or not.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			ctx := spec.FixtureFactory.Context()

			var err error
			ctx, err = spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ctx, err = spec.Subject.BeginTx(ctx)
			require.Nil(t, err)

			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Create(ctx, entity))
			id, ok := resources.LookupID(entity)
			require.True(t, ok)
			require.NotEmpty(t, id)
			t.Defer(spec.Subject.DeleteByID, spec.FixtureFactory.Context(), spec.EntityType, id)

			require.Nil(t, spec.Subject.CommitTx(ctx), `"inner" tx should be considered done`)
			require.Nil(t, spec.Subject.CommitTx(ctx), `"outer" tx should be considered done`)

			t.Log(`after everything is committed, the stored entity should be findable`)
			found, err := spec.Subject.FindByID(spec.FixtureFactory.Context(), spec.newEntity(), id)
			require.Nil(t, err)
			require.True(t, found)
		})
	})
}

func (spec OnePhaseCommitProtocolSpec) newEntity() interface{} {
	return reflect.New(reflect.TypeOf(spec.EntityType)).Interface()
}
