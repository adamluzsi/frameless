package resourcespecs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
)

type Purge interface {
	Purge() error
}

type PurgeSpec struct {
	Subject interface {
		Purge
		Save
		FindByID
	}
}

func (q PurgeSpec) Test(t *testing.T, r resources.Resource) {
	t.Run("purge out all data from the given resource", func(t *testing.T) {

		fixture := newFixture(unexportedEntity{})
		err := q.Subject.Save(fixture)
		id, ok := LookupID(fixture)

		require.True(t, ok)
		require.NotEmpty(t, id)
		require.Nil(t, err)

		var value unexportedEntity
		ok, err = q.Subject.FindByID(id, &value)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, fixture, &value)

		require.Nil(t, q.Subject.Purge())

		ok, err = q.Subject.FindByID(id, &unexportedEntity{})
		require.Nil(t, err)
		require.False(t, ok)

	})
}
