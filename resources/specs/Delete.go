package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"

	"github.com/stretchr/testify/require"
)

type Delete interface {
	Delete(ctx context.Context, Entity interface{}) error
}

// DeleteSpec request a destroy of a specific entity that is wrapped in the query use case object
type DeleteSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject iDelete
}

type iDelete interface {
	Delete

	MinimumRequirements
}

// Test will test that an DeleteSpec is implemented by a generic specification
func (spec DeleteSpec) Test(t *testing.T) {

	if _, hasExtID := LookupID(spec.EntityType); !hasExtID {
		t.Fatalf(
			`entity type that doesn't include external resource ID field is not compatible with this contract (%s)`,
			reflects.FullyQualifiedName(spec.EntityType),
		)
	}

	expected := spec.FixtureFactory.Create(spec.EntityType)
	require.Nil(t, spec.Subject.Save(spec.Context(), expected))
	ID, ok := LookupID(expected)

	if !ok {
		t.Fatal(frameless.ErrIDRequired)
	}

	defer spec.Subject.DeleteByID(spec.Context(), reflects.BaseValueOf(spec.EntityType).Interface(), ID)

	t.Run("value is Deleted by providing an EntityType, and then it should not be findable afterwards", func(t *testing.T) {

		err := spec.Subject.Delete(spec.Context(), expected)
		require.Nil(t, err)

		e := spec.FixtureFactory.Create(spec.EntityType)
		ok, err := spec.Subject.FindByID(spec.Context(), e, ID)
		require.Nil(t, err)
		require.False(t, ok)

	})
}

func TestDelete(t *testing.T, r iDelete, e interface{}, f FixtureFactory) {
	t.Run(`Delete`, func(t *testing.T) {
		DeleteSpec{EntityType: e, FixtureFactory: f, Subject: r}.Test(t)
	})
}
