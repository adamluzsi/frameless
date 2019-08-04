package specs

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"testing"

	"github.com/stretchr/testify/require"
)

type FindByID interface {
	FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error)
}

type FindByIDSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec FindByIDSpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`FindByID`, func(s *testcase.Spec) {

		subject := func(t *testcase.T) (bool, error) {
			return spec.Subject.FindByID(
				t.I(`ctx`).(context.Context),
				t.I(`ptr`),
				t.I(`id`).(string),
			)
		}

		s.Let(`ctx`, func(t *testcase.T) interface{} {
			return spec.Context()
		})

		s.Let(`ptr`, func(t *testcase.T) interface{} {
			return reflects.New(spec.EntityType)
		})

		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
		})

		s.Let(`entity`, func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.EntityType)
		})

		s.When(`entity was saved in the resource`, func(s *testcase.Spec) {

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`entity`)))
			})

			s.Let(`id`, func(t *testcase.T) interface{} {
				id, ok := LookupID(t.I(`entity`))
				require.True(t, ok)
				return id
			})

			s.Then(`the entity will be returned`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.True(t, found)
				require.Equal(t, t.I(`entity`), t.I(`ptr`))
			})

			s.And(`more similar entity is saved in the resource as well`, func(s *testcase.Spec) {
				s.Let(`oth-entity`, func(t *testcase.T) interface{} {
					return spec.FixtureFactory.Create(spec.EntityType)
				})
				s.Before(func(t *testcase.T) {
					require.Nil(t, spec.Subject.Save(spec.Context(), t.I(`oth-entity`)))
				})

				s.Then(`the entity`, func(t *testcase.T) {
					found, err := subject(t)
					require.Nil(t, err)
					require.True(t, found)
					require.Equal(t, t.I(`entity`), t.I(`ptr`))
				})
			})
		})

		s.When(`no entity saved before in the resource`, func(s *testcase.Spec) {
			s.Let(`id`, func(t *testcase.T) interface{} { return `` })

			s.Before(func(t *testcase.T) {
				require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))
			})

			s.Then(`the iterator will have no result`, func(t *testcase.T) {
				found, err := subject(t)
				require.Nil(t, err)
				require.False(t, found)
			})
		})

		s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
			s.Let(`id`, func(t *testcase.T) interface{} { return `` })

			s.Let(`ctx`, func(t *testcase.T) interface{} {
				ctx, cancel := context.WithCancel(spec.Context())
				cancel()
				return ctx
			})

			s.Then(`it expected to return with context cancel error`, func(t *testcase.T) {
				found, err := subject(t)
				require.Equal(t, context.Canceled, err)
				require.False(t, found)
			})
		})
	})

	s.Test(`E2E`, func(t *testcase.T) {
		var ids []string

		for i := 0; i < 12; i++ {

			entity := spec.FixtureFactory.Create(spec.EntityType)

			require.Nil(t, spec.Subject.Save(spec.Context(), entity))
			ID, ok := LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)
			ids = append(ids, ID)

		}

		defer func() {
			for _, id := range ids {
				require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.EntityType, id))
			}
		}()

		t.Run("when no value stored that the query request", func(t *testing.T) {
			ptr := reflects.New(spec.EntityType)

			ok, err := spec.Subject.FindByID(spec.Context(), ptr, "not existing ID")

			require.Nil(t, err)
			require.False(t, ok)
		})

		t.Run("values returned", func(t *testing.T) {
			for _, ID := range ids {

				entityPtr := reflects.New(spec.EntityType)
				ok, err := spec.Subject.FindByID(spec.Context(), entityPtr, ID)

				require.Nil(t, err)
				require.True(t, ok)

				actualID, ok := LookupID(entityPtr)

				if !ok {
					t.Fatal("can't find ID in the returned value")
				}

				require.Equal(t, ID, actualID)

			}
		})
	})

}

func TestFindByID(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	FindByIDSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
}
