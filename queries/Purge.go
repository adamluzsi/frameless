package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
	"testing"
)

type Purge struct{}

func (q Purge) Test(t *testing.T, r frameless.Resource) {
	t.Run("purge out all data from the given resource", func(t *testing.T) {

		fixture := fixtures.New(unexportedEntity{})
		res := r.Exec(SaveEntity{fixture})
		id, ok := resources.LookupID(fixture)

		require.True(t, ok)
		require.NotEmpty(t, id)
		require.NotNil(t, res)
		require.Nil(t, res.Err())

		var value unexportedEntity
		i := r.Exec(FindByID{Type: unexportedEntity{}, ID: id})
		require.NotNil(t, i)
		require.True(t, i.Next())
		require.Nil(t, i.Decode(&value))
		require.Equal(t, fixture, &value)

		i = r.Exec(Purge{})
		require.NotNil(t, i)
		require.Nil(t, i.Err())

		i = r.Exec(FindByID{Type: unexportedEntity{}, ID: id})
		require.NotNil(t, i)
		require.False(t, i.Next())

	})
}
