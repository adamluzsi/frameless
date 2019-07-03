package specs

import (
	"context"
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/stretchr/testify/require"
)

type Purge interface {
	Purge(ctx context.Context) error
}

type PurgeSpec struct {
	Subject iPurge
	FixtureFactory
	EntityType interface{}
}

type iPurge interface {
	Purge

	MinimumRequirements
}

func (spec PurgeSpec) Test(t *testing.T) {
	t.Run("purge out all data from the given resource", func(t *testing.T) {

		fixture := spec.FixtureFactory.Create(spec.EntityType)
		err := spec.Subject.Save(spec.Context(spec.EntityType), fixture)
		id, ok := LookupID(fixture)

		require.True(t, ok)
		require.NotEmpty(t, id)
		require.Nil(t, err)

		value := reflects.New(spec.EntityType)
		ok, err = spec.Subject.FindByID(spec.Context(spec.EntityType), value, id)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, fixture, value)

		require.Nil(t, spec.Subject.Purge(spec.Context(spec.EntityType)))

		ok, err = spec.Subject.FindByID(spec.Context(spec.EntityType), reflects.New(spec.EntityType), id)
		require.Nil(t, err)
		require.False(t, ok)

	})
}

func TestPurge(t *testing.T, r iPurge, e interface{}, f FixtureFactory) {
	t.Run(`Purge`, func(t *testing.T) {
		PurgeSpec{Subject: r, EntityType: e, FixtureFactory: f}.Test(t)
	})
}
