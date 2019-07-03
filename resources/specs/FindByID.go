package specs

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/adamluzsi/frameless"

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

	var ids []string

	for i := 0; i < 12; i++ {

		entity := spec.FixtureFactory.Create(spec.EntityType)

		require.Nil(t, spec.Subject.Save(spec.Context(spec.EntityType), entity))
		ID, ok := LookupID(entity)

		if !ok {
			t.Fatal(frameless.ErrIDRequired)
		}

		require.True(t, len(ID) > 0)
		ids = append(ids, ID)

	}

	defer func() {
		for _, id := range ids {
			require.Nil(t, spec.Subject.DeleteByID(spec.Context(spec.EntityType), spec.EntityType, id))
		}
	}()

	t.Run("when no value stored that the query request", func(t *testing.T) {
		ptr := reflects.New(spec.EntityType)

		ok, err := spec.Subject.FindByID(spec.Context(spec.EntityType), ptr, "not existing ID")

		require.Nil(t, err)
		require.False(t, ok)
	})

	t.Run("values returned", func(t *testing.T) {
		for _, ID := range ids {

			entityPtr := reflects.New(spec.EntityType)
			ok, err := spec.Subject.FindByID(spec.Context(spec.EntityType), entityPtr, ID)

			require.Nil(t, err)
			require.True(t, ok)

			actualID, ok := LookupID(entityPtr)

			if !ok {
				t.Fatal("can't find ID in the returned value")
			}

			require.Equal(t, ID, actualID)

		}
	})

}

func TestFindByID(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	t.Run(`FindByID`, func(t *testing.T) {
		FindByIDSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})
}
