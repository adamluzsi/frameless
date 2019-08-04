package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// DeleteByID request to destroy a business entity in the resource that implement it's test.
type DeleteByID interface {
	DeleteByID(ctx context.Context, Type interface{}, ID string) error
}

type DeleteByIDSpec struct {
	EntityType interface {}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec DeleteByIDSpec) Test(t *testing.T) {

	t.Run("given database is populated", func(t *testing.T) {
		var ids []string

		for i := 0; i < 10; i++ {

			entity := spec.FixtureFactory.Create(spec.EntityType)
			require.Nil(t, spec.Subject.Save(spec.Context(), entity))
			ID, ok := LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)
			ids = append(ids, ID)

		}

		t.Run("using delete by id makes entity with ID not find-able", func(t *testing.T) {
			for _, ID := range ids {
				e := spec.FixtureFactory.Create(spec.EntityType)

				ok, err := spec.Subject.FindByID(spec.Context(), e, ID)
				require.True(t, ok)
				require.Nil(t, err)

				err = spec.Subject.DeleteByID(spec.Context(), e, ID)
				require.Nil(t, err)

				ok, err = spec.Subject.FindByID(spec.Context(), e, ID)
				require.Nil(t, err)
				require.False(t, ok)

			}
		})
	})

}

func TestDeleteByID(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	t.Run(`DeleteByID`, func(t *testing.T) {
		DeleteByIDSpec{Subject:r, EntityType: e, FixtureFactory: f}.Test(t)
	})
}