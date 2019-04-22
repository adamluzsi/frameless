package queries

import (
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/stretchr/testify/require"
)

type Truncate struct{ Type interface{} }

func (query Truncate) Test(t *testing.T, r resources.Resource) {

	populateFor := func(t *testing.T, Type interface{}) string {
		fixture := newFixture(Type)
		require.Nil(t, r.Exec(Save{Entity: fixture}).Err())

		id, ok := LookupID(fixture)
		require.True(t, ok)
		require.NotEmpty(t, id)

		return id
	}

	isStored := func(t *testing.T, ID string, Type interface{}) bool {
		iter := r.Exec(FindByID{Type: Type, ID: ID})
		require.NotNil(t, iter)
		require.Nil(t, iter.Err())

		count, err := iterators.Count(iter)
		require.Nil(t, err)

		return count == 1
	}

	t.Run("delete all records based on what entity object it receives", func(t *testing.T) {

		eID := populateFor(t, query.Type)
		oID := populateFor(t, TruncateTestEntity{})

		require.True(t, isStored(t, eID, query.Type))
		require.True(t, isStored(t, oID, TruncateTestEntity{}))

		require.Nil(t, r.Exec(Truncate{Type: query.Type}).Err())

		require.False(t, isStored(t, eID, query.Type))
		require.True(t, isStored(t, oID, TruncateTestEntity{}))

		require.Nil(t, r.Exec(DeleteByID{Type: TruncateTestEntity{}, ID: oID}).Err())

	})
}

type TruncateTestEntity struct {
	ID string `ext:"ID"`
}
