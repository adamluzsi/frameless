package specs

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type Update interface {
	Update(ctx context.Context, ptr interface{}) error
}

// UpdateSpec will request an update for a wrapped entity object in the resource
type UpdateSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject iUpdate
}

type iUpdate interface {
	Update

	MinimumRequirements
}

func (spec UpdateSpec) Test(suite *testing.T) {
	suite.Run("Update", func(t *testing.T) {

		if _, hasExtID := LookupID(spec.EntityType); !hasExtID {
			t.Fatalf(
				`entity type that doesn't include external resource ID field is not compatible with this contract (%s)`,
				reflects.FullyQualifiedName(spec.EntityType),
			)
		}

		setup := func(t *testing.T) (string, func()) {
			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Save(spec.Context(), entity))

			ID, ok := LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)

			td := func() { require.Nil(t, spec.Subject.DeleteByID(spec.Context(), spec.EntityType, ID)) }

			return ID, td
		}

		t.Run("values returned", func(t *testing.T) {
			ID, td := setup(t)
			defer td()

			newEntity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, SetID(newEntity, ID))

			err := spec.Subject.Update(spec.Context(), newEntity)
			require.Nil(t, err)

			actually := reflects.New(spec.EntityType)
			ok, err := spec.Subject.FindByID(spec.Context(), actually, ID)
			require.True(t, ok)
			require.Nil(t, err)

			require.Equal(t, newEntity, actually)

		})

		t.Run("when request id does not exist it will return with error", func(t *testing.T) {
			_, td := setup(t)
			defer td()

			newEntity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, SetID(newEntity, "hitchhiker's guide to the galaxy"))
			require.Error(t, spec.Subject.Update(spec.Context(), newEntity))
		})

	})
}

func TestUpdate(t *testing.T, r iUpdate, e interface{}, f FixtureFactory) {
	t.Run(`Update`, func(t *testing.T) {
		UpdateSpec{EntityType: e, FixtureFactory: f, Subject: r}.Test(t)
	})
}
