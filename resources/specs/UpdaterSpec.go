package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

// Updater will request an update for a wrapped entity object in the Resource
type Updater struct {
	T interface{}
	FixtureFactory
	Subject interface {
		resources.Updater
		minimumRequirements
	}
}

func (spec Updater) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		require.Nil(t, spec.Subject.DeleteAll(spec.Context(), spec.T))
	})

	thenExternalIDFieldIsExpected(s, spec.T)

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
				return spec.FixtureFactory.Create(spec.T)
			})

			s.Let(`entity.id`, func(t *testcase.T) interface{} {
				id, ok := resources.LookupID(t.I(`entity`))
				require.True(t, ok, ErrIDRequired)
				return id
			})

			s.Around(func(t *testcase.T) func() {
				entity := t.I(`entity`)
				require.Nil(t, spec.Subject.Create(spec.Context(), entity))
				return func() {
					id, _ := resources.LookupID(entity)
					require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.T, id))
				}
			})

			s.And(`and the received entity in argument use the stored entity's ext.ID`, func(s *testcase.Spec) {
				s.Let(`entity-with-changes`, func(t *testcase.T) interface{} {
					newEntity := spec.FixtureFactory.Create(spec.T)
					id, _ := resources.LookupID(t.I(`entity`))
					require.Nil(t, resources.SetID(newEntity, id))
					return newEntity
				})

				s.Then(`then it will update stored entity values by the received one`, func(t *testcase.T) {
					require.Nil(t, subject(t))

					id := t.I(`entity.id`).(string)
					actually := newEntityBasedOn(spec.T)
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
				newEntity := spec.FixtureFactory.Create(spec.T)
				require.Nil(t, resources.SetID(newEntity, fixtures.Random.String()))
				return newEntity
			})

			s.Then(`it will encounter error during the update of the stored entity`, func(t *testcase.T) {
				require.Error(t, subject(t))
			})
		})

	})
}

func (spec Updater) Benchmark(b *testing.B) {
	cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)
	b.Run(`Updater`, func(b *testing.B) {
		es := createEntities(spec.FixtureFactory, spec.T)
		saveEntities(b, spec.Subject, spec.FixtureFactory, es...)
		defer cleanup(b, spec.Subject, spec.FixtureFactory, spec.T)

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
