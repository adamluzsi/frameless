package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"

	"github.com/stretchr/testify/require"
)

// UpdaterSpec will request an update for a wrapped entity object in the Resource
type UpdaterSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject updateSpecSubject
}

type updateSpecSubject interface {
	resources.Updater

	MinimumRequirements
}

func (spec UpdaterSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	extIDFieldRequired(s, spec.EntityType)

	s.Describe(`Updater`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) error {
			return spec.Subject.Update(
				t.I(`ctx`).(context.Context),
				t.I(`entity-with-changes`),
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.When(`an entity already stored`, func(s *testcase.Spec) {

			s.Let(`entity`, func(t *testcase.T) interface{} {
				return spec.FixtureFactory.Create(spec.EntityType)
			})

			s.Let(`entity.id`, func(t *testcase.T) interface{} {
				id, ok := resources.LookupID(t.I(`entity`))
				require.True(t, ok, frameless.ErrIDRequired)
				return id
			})

			s.Around(func(t *testcase.T) func() {
				entity := t.I(`entity`)
				require.Nil(t, spec.Subject.Save(spec.Context(), entity))
				return func() {
					id, _ := resources.LookupID(entity)
					require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.EntityType, id))
				}
			})

			s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
				s.Let(`entity-with-changes`, func(t *testcase.T) interface{} {
					newEntity := spec.FixtureFactory.Create(spec.EntityType)
					id, _ := resources.LookupID(t.I(`entity`))
					require.Nil(t, resources.SetID(newEntity, id))
					return newEntity
				})

				s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
					require.Nil(t, subject(t))

					id := t.I(`entity.id`).(string)
					actually := reflects.New(spec.EntityType)
					ok, err := spec.Subject.FindByID(spec.Context(), actually, id)
					require.True(t, ok)
					require.Nil(t, err)

					require.Equal(t, t.I(`entity-with-changes`), actually)
				})

				s.And(`ctx arg is canceled`, func(s *testcase.Spec) {
					s.Let(`ctx`, func(t *testcase.T) interface{} {
						ctx, cancel := context.WithCancel(spec.Context())
						cancel()
						return ctx
					})

					s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
						require.Equal(t, context.Canceled, subject(t))
					})
				})
			})
		})

		s.When(`the received entity has ext.ID that is unknown in the storage`, func(s *testcase.Spec) {
			s.Let(`entity-with-changes`, func(t *testcase.T) interface{} {
				newEntity := spec.FixtureFactory.Create(spec.EntityType)
				require.Nil(t, resources.SetID(newEntity, fixtures.RandomString(42)))
				return newEntity
			})

			s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

	})
}

func (spec UpdaterSpec) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)
	b.Run(`UpdaterSpec`, func(b *testing.B) {
		es := createEntities(benchmarkEntityVolumeCount, spec.FixtureFactory, spec.EntityType)
		 saveEntities(b, spec.Subject, spec.FixtureFactory, es...)
		defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.EntityType)

		var executionTimes int
		b.ResetTimer()
	wrk:
		for {
			for _, ptr := range es {
				require.Nil(b, spec.Subject.Update(spec.Context(), ptr))

				executionTimes++
				if b.N <= executionTimes {
					break wrk
				}
			}
		}
	})
}
