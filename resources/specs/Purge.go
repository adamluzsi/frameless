package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/stretchr/testify/require"
)

type Purge interface {
	Purge() error
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
		err := spec.Subject.Save(fixture)
		id, ok := LookupID(fixture)

		require.True(t, ok)
		require.NotEmpty(t, id)
		require.Nil(t, err)

		value := reflects.New(spec.EntityType)
		ok, err = spec.Subject.FindByID(id, value)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, fixture, value)

		require.Nil(t, spec.Subject.Purge())

		ok, err = spec.Subject.FindByID(id, reflects.New(spec.EntityType))
		require.Nil(t, err)
		require.False(t, ok)

	})
}

func TestPurge(t *testing.T, r iPurge, e interface{}, f FixtureFactory) {
	t.Run(`Purge`, func(t *testing.T) {
		PurgeSpec{Subject: r, EntityType: e, FixtureFactory: f}.Test(t)
	})
}
