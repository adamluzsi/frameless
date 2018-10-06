package save

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"
	"testing"
)

type Entity struct {
	Entity frameless.Entity
}

func (q Entity) Test(t *testing.T, s frameless.Storage, resetDB func()) {
	t.Run("persist an Entity", func(t *testing.T) {

		if ID, _ := storages.LookupID(q.Entity); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		e := fixtures.New(q.Entity)
		i := s.Exec(Entity{Entity: e})

		require.NotNil(t, i)
		require.Nil(t, i.Err())

		ID, ok := storages.LookupID(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.True(t, len(ID) > 0, "it's expected that storage set the storage ID in the entity")

	})

	t.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		defer resetDB()

		newEntity := fixtures.New(entityWithoutIDField{})
		require.Error(t, s.Exec(Entity{Entity: newEntity}).Err())
	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		defer resetDB()

		newEntity := fixtures.New(q.Entity)
		storages.SetID(newEntity, "Hello world!")
		require.Error(t, s.Exec(Entity{Entity: newEntity}).Err())
	})
}

type entityWithoutIDField struct {
	Data string
}
