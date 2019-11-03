package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type TruncaterSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec TruncaterSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`Truncater`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) error {
			return spec.Subject.Truncate(
				t.I(`ctx`).(context.Context),
				spec.EntityType,
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} { return spec.Context() })

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				require.Equal(t, context.Canceled, subject(t))
			})
		})

		s.Then(`it should remove all entities from the resource`, func(t *testcase.T) {
			eID := spec.populateFor(t, spec.EntityType)
			require.True(t, spec.isStored(t, eID, spec.EntityType))
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
			require.False(t, spec.isStored(t, eID, spec.EntityType))
		})

		s.Then(`it should not affect other entities`, func(t *testcase.T) {
			t.Skip(`TODO/POC`)

			ff, ok := spec.FixtureFactory.(interface {
				OthEntityType(e interface{}) interface{}
			})
			if !ok {
				t.Skip(`OthEntityType not yet implemented`)
			}

			spec.populateFor(t, spec.EntityType)
			othT := ff.OthEntityType(spec.EntityType)
			oID := spec.populateFor(t, othT)

			require.True(t, spec.isStored(t, oID, othT))
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
			require.True(t, spec.isStored(t, oID, othT))
			require.Nil(t, spec.Subject.DeleteByID(spec.Context(), othT, oID))
		})
	})
}

func (spec TruncaterSpec) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)
	b.Run(`TruncaterSpec`, func(b *testing.B) {
		// for some reason, doing setup with timer stop/start
		// makes this test unable to measure
		// the correct throughput, and hangs forever
		// so I just check empty db truncate then.
		// This anyway not a thing that is often used.
		for i := 0; i < b.N; i++ {
			require.Nil(b, spec.Subject.Truncate(spec.Context(), spec.EntityType))
		}
	})
}

func (spec TruncaterSpec) populateFor(t testing.TB, Type interface{}) string {
	fixture := spec.FixtureFactory.Create(Type)
	require.Nil(t, spec.Subject.Save(spec.Context(), fixture))

	id, ok := resources.LookupID(fixture)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func (spec TruncaterSpec) isStored(t testing.TB, ID string, Type interface{}) bool {
	entity := newEntityBasedOn(Type)
	ok, err := spec.Subject.FindByID(spec.Context(), entity, ID)
	require.Nil(t, err)
	return ok
}
