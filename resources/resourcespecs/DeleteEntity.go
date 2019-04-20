package resourcespecs

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"

	"github.com/stretchr/testify/require"
)

type Delete interface {
	Delete(interface {}) error
}

// DeleteSpec request a destroy of a specific entity that is wrapped in the query use case object
type DeleteSpec struct {
	Entity interface {}

	Subject interface {
		Save
		Delete
		DeleteByID
		FindByID
	}
}

// Test will test that an DeleteSpec is implemented by a generic specification
func (quc DeleteSpec) Test(spec *testing.T) {

	expected := newFixture(quc.Entity)
	require.Nil(spec, quc.Subject.Save(expected))
	ID, ok := LookupID(expected)

	if !ok {
		spec.Fatal(frameless.ErrIDRequired)
	}

	defer quc.Subject.DeleteByID(reflects.BaseValueOf(quc.Entity).Interface(), ID)

	spec.Run("value is Deleted by providing an Entity, and then it should not be findable afterwards", func(t *testing.T) {

		err := quc.Subject.Delete(expected)
		require.Nil(t, err)

		e := newFixture(quc.Entity)
		ok, err := quc.Subject.FindByID(ID, e)
		require.Nil(t, err)
		require.False(t, ok)

	})

	spec.Run("when entity doesn't have r ID field", func(t *testing.T) {
		newEntity := newFixture(entityWithoutIDField{})

		require.Error(t, quc.Subject.Delete(newEntity))
	})
}
