package specs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type Save interface {
	Save(interface{}) error
}

type SaveSpec struct {
	Type interface{}

	Subject MinimumRequirements
}

func (spec SaveSpec) Test(t *testing.T) {
	t.Run("persist an Save", func(t *testing.T) {

		if ID, _ := LookupID(spec.Type); ID != "" {
			t.Fatalf("expected entity shouldn't have any ID yet, but have %s", ID)
		}

		e := newFixture(spec.Type)
		err := spec.Subject.Save(e)

		require.Nil(t, err)

		ID, ok := LookupID(e)
		require.True(t, ok, "ID is not defined in the entity struct src definition")
		require.NotEmpty(t, ID, "it's expected that storage set the storage ID in the entity")

		actual := newFixture(spec.Type)

		ok, err = spec.Subject.FindByID(ID, actual)
		require.True(t, ok)
		require.Nil(t, err)
		require.Equal(t, e, actual)

		require.Nil(t, spec.Subject.DeleteByID(spec.Type, ID))

	})

	t.Run("when entity doesn't have storage ID field", func(t *testing.T) {
		newEntity := newFixture(entityWithoutIDField{})

		require.Error(t, spec.Subject.Save(newEntity))
	})

	t.Run("when entity already have an ID", func(t *testing.T) {
		newEntity := newFixture(spec.Type)
		SetID(newEntity, "Hello world!")

		require.Error(t, spec.Subject.Save(newEntity))
	})
}

func TestSave(t *testing.T, r MinimumRequirements, e interface{}) {
	t.Run(`Save`, func(t *testing.T) {
		SaveSpec{Type: e, Subject: r}.Test(t)
	})
}
