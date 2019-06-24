package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/stretchr/testify/require"
)

type Save interface {
	Save(interface{}) error
}

type SaveSpec struct {
	EntityType interface{}
	FixtureFactory
	Subject MinimumRequirements
}

func (spec SaveSpec) Test(t *testing.T) {

	if _, hasExtID := LookupID(spec.EntityType); !hasExtID {
		t.Fatalf(
			`entity type that doesn't include external resource ID field is not compatible with this contract (%s)`,
			reflects.FullyQualifiedName(spec.EntityType),
		)
	}

	t.Run("persist an Save", func(t *testing.T) {

		if ID, _ := LookupID(spec.EntityType); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		e := spec.FixtureFactory.Create(spec.EntityType)
		err := spec.Subject.Save(e)

		require.Nil(t, err)

		ID, ok := LookupID(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

		actual := spec.FixtureFactory.Create(spec.EntityType)

		ok, err = spec.Subject.FindByID(ID, actual)
		require.Nil(t, err)
		require.True(t, ok)
		require.Equal(t, e, actual)

		require.Nil(t, spec.Subject.DeleteByID(spec.EntityType, ID))

	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		newEntity := spec.FixtureFactory.Create(spec.EntityType)
		require.Nil(t, SetID(newEntity, "Hello world!"))
		require.Error(t, spec.Subject.Save(newEntity))
	})
}

func TestSave(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	t.Run(`Save`, func(t *testing.T) {
		SaveSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})
}
