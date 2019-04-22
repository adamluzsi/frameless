package specs

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"

	"github.com/stretchr/testify/require"
)

type Delete interface {
	Delete(Entity interface{}) error
}

type iDelete interface {
	Delete

	MinimumRequirements
}

// DeleteSpec request a destroy of a specific entity that is wrapped in the query use case object
type DeleteSpec struct {
	Entity interface{}

	Subject iDelete
}

// Test will test that an DeleteSpec is implemented by a generic specification
func (spec DeleteSpec) Test(t *testing.T) {

	expected := newFixture(spec.Entity)
	require.Nil(t, spec.Subject.Save(expected))
	ID, ok := LookupID(expected)

	if !ok {
		t.Fatal(frameless.ErrIDRequired)
	}

	defer spec.Subject.DeleteByID(reflects.BaseValueOf(spec.Entity).Interface(), ID)

	t.Run("value is Deleted by providing an Type, and then it should not be findable afterwards", func(t *testing.T) {

		err := spec.Subject.Delete(expected)
		require.Nil(t, err)

		e := newFixture(spec.Entity)
		ok, err := spec.Subject.FindByID(ID, e)
		require.Nil(t, err)
		require.False(t, ok)

	})

	t.Run("when entity doesn't have r ID field", func(t *testing.T) {
		newEntity := newFixture(entityWithoutIDField{})

		require.Error(t, spec.Subject.Delete(newEntity))
	})
}


func TestDelete(t *testing.T, r iDelete, e interface{}) {
	t.Run(`Delete`, func(t *testing.T) {
		DeleteSpec{Entity: e, Subject: r}.Test(t)
	})
}
