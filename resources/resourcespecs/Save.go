package resourcespecs

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

type Save interface {
	Save(frameless.Entity) error
}

type SaveSpec struct {
	Entity frameless.Entity

	Subject interface {
		Save
		FindByID
		DeleteByID
	}
}

func (q SaveSpec) Test(t *testing.T) {
	Type := reflects.BaseValueOf(q.Entity).Interface()

	t.Run("persist an Save", func(t *testing.T) {

		if ID, _ := LookupID(q.Entity); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		e := newFixture(Type)
		err := q.Subject.Save(e)

		require.Nil(t, err)

		ID, ok := LookupID(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

		actual := newFixture(Type)

		ok, err = q.Subject.FindByID(ID, actual)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, e, actual)

		require.Nil(t, q.Subject.DeleteByID(Type, ID))

	})

	t.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		newEntity := newFixture(entityWithoutIDField{})

		require.Error(t, q.Subject.Save(newEntity))
	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		newEntity := newFixture(Type)
		SetID(newEntity, "Hello world!")

		require.Error(t, q.Subject.Save(newEntity))
	})
}
