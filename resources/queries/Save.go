package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
	"github.com/stretchr/testify/require"
	"testing"
)

type Save struct {
	Entity frameless.Entity
}

func (q Save) Test(t *testing.T, r resources.Resource) {
	Type := reflects.BaseValueOf(q.Entity).Interface()

	t.Run("persist an Save", func(t *testing.T) {

		if ID, _ := LookupID(q.Entity); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		expected := newFixture(q.Entity)
		i := r.Exec(Save{Entity: expected})

		require.NotNil(t, i)
		require.Nil(t, i.Err())

		ID, ok := LookupID(expected)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it was expected that Save set the storage ID in the entity")

		i = r.Exec(FindByID{Type: Type, ID: ID})
		actual := reflects.New(Type)
		require.Nil(t, iterators.DecodeNext(i, actual))
		require.Equal(t, expected, actual)
		require.Nil(t, r.Exec(DeleteByID{Type: Type, ID: ID}).Err())

	})

	t.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		newEntity := newFixture(entityWithoutIDField{})
		require.Error(t, r.Exec(Save{Entity: newEntity}).Err())
	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		newEntity := newFixture(q.Entity)
		SetID(newEntity, "Hello world!")
		require.Error(t, r.Exec(Save{Entity: newEntity}).Err())
	})
}
