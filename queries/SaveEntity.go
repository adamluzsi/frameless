package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries/fixtures"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
	"testing"
)

type SaveEntity struct {
	Entity frameless.Entity
}

func (q SaveEntity) Test(t *testing.T, r frameless.Resource) {
	Type := reflects.BaseValueOf(q.Entity).Interface()

	t.Run("persist an SaveEntity", func(t *testing.T) {

		if ID, _ := resources.LookupID(q.Entity); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		e := fixtures.New(q.Entity)
		i := r.Exec(SaveEntity{Entity: e})

		require.NotNil(t, i)
		require.Nil(t, i.Err())

		ID, ok := resources.LookupID(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

		actual := fixtures.New(q.Entity)

		i = r.Exec(FindByID{Type: Type, ID: ID})
		require.Nil(t, iterators.DecodeNext(i, actual))
		require.Equal(t, e, actual)
		require.Nil(t, r.Exec(DeleteByID{Type: Type, ID: ID}).Err())


	})

	t.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		newEntity := fixtures.New(entityWithoutIDField{})
		require.Error(t, r.Exec(SaveEntity{Entity: newEntity}).Err())
	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		newEntity := fixtures.New(q.Entity)
		resources.SetID(newEntity, "Hello world!")
		require.Error(t, r.Exec(SaveEntity{Entity: newEntity}).Err())
	})
}
