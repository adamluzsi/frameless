package specs

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/stretchr/testify/require"
)

type Truncate interface {
	Truncate(ctx context.Context, Type interface{}) error
}

type TruncateSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec TruncateSpec) Test(t *testing.T) {

	populateFor := func(t *testing.T, Type interface{}) string {
		fixture := spec.FixtureFactory.Create(Type)
		require.Nil(t, spec.Subject.Save(spec.Context(), fixture))

		id, ok := LookupID(fixture)
		require.True(t, ok)
		require.NotEmpty(t, id)

		return id
	}

	isStored := func(t *testing.T, ID string, Type interface{}) bool {
		entity := reflects.New(Type)
		ok, err := spec.Subject.FindByID(spec.Context(), entity, ID)
		require.Nil(t, err)
		return ok
	}

	t.Run("delete all records based on what entity object it receives", func(t *testing.T) {

		eID := populateFor(t, spec.EntityType)
		oID := populateFor(t, TestEntity{})

		require.True(t, isStored(t, eID, spec.EntityType))
		require.True(t, isStored(t, oID, TestEntity{}))

		require.Nil(t, spec.Subject.Truncate(spec.Context(), spec.EntityType))

		require.False(t, isStored(t, eID, spec.EntityType))
		require.True(t, isStored(t, oID, TestEntity{}))

		require.Nil(t, spec.Subject.DeleteByID(spec.Context(), TestEntity{}, oID))

	})
}

func TestTruncate(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	t.Run(`Truncate`, func(t *testing.T) {
		TruncateSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})
}

