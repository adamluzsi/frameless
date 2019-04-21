package specs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type Purge interface {
	Purge() error
}

type PurgeSpec struct {
	Subject interface {
		Purge

		MinimumRequirements
	}
}

func (spec PurgeSpec) Test(t *testing.T) {
	t.Run("purge out all data from the given resource", func(t *testing.T) {

		fixture := newFixture(unexportedEntity{})
		err := spec.Subject.Save(fixture)
		id, ok := LookupID(fixture)

		require.True(t, ok)
		require.NotEmpty(t, id)
		require.Nil(t, err)

		var value unexportedEntity
		ok, err = spec.Subject.FindByID(id, &value)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, fixture, &value)

		require.Nil(t, spec.Subject.Purge())

		ok, err = spec.Subject.FindByID(id, &unexportedEntity{})
		require.Nil(t, err)
		require.False(t, ok)

	})
}
