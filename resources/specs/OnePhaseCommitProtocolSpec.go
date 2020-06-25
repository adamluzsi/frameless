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
	spec.spec(t)
}

func (spec OnePhaseCommitProtocolSpec) Benchmark(b *testing.B) {
	spec.spec(b)
}

func (spec OnePhaseCommitProtocolSpec) spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	s.HasSideEffect()

	s.Context(`OnePhaseCommitProtocolSpec`, func(s *testcase.Spec) {
		s.Around(func(t *testcase.T) func() {
			clean := func() {
				require.Nil(t, spec.Subject.DeleteAll(spec.FixtureFactory.Context(), spec.EntityType))
			}
			clean()
			return clean
		})

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

		s.Test(`BeginTx twice on the same context yields error`, func(t *testcase.T) {
			ctx := spec.FixtureFactory.Context()
			t.Defer(spec.Subject.RollbackTx, ctx)

			var err error
			ctx, err = spec.Subject.BeginTx(ctx)
			require.Nil(t, err)
			ctx, err = spec.Subject.BeginTx(ctx)
			require.Error(t, err)
		})
	})
}

func (spec OnePhaseCommitProtocolSpec) newEntity() interface{} {
	return reflect.New(reflect.TypeOf(spec.EntityType)).Interface()
}
