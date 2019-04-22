package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"testing"

	"github.com/stretchr/testify/require"
)

type Truncate interface {
	Truncate(Type interface{}) error
}

type TruncateSpec struct {
	Type    interface{}
	Subject MinimumRequirements
}

func (spec TruncateSpec) Test(t *testing.T) {

	populateFor := func(t *testing.T, Type interface{}) string {
		fixture := newFixture(Type)
		require.Nil(t, spec.Subject.Save(fixture))

		id, ok := LookupID(fixture)
		require.True(t, ok)
		require.NotEmpty(t, id)

		return id
	}

	isStored := func(t *testing.T, ID string, Type interface{}) bool {
		entity := reflects.New(Type)
		ok, err := spec.Subject.FindByID(ID, entity)
		require.Nil(t, err)
		return ok
	}

	t.Run("delete all records based on what entity object it receives", func(t *testing.T) {

		eID := populateFor(t, spec.Type)
		oID := populateFor(t, TruncateTestEntity{})

		require.True(t, isStored(t, eID, spec.Type))
		require.True(t, isStored(t, oID, TruncateTestEntity{}))

		require.Nil(t, spec.Subject.Truncate(spec.Type))

		require.False(t, isStored(t, eID, spec.Type))
		require.True(t, isStored(t, oID, TruncateTestEntity{}))

		require.Nil(t, spec.Subject.DeleteByID(TruncateTestEntity{}, oID))

	})
}

func TestTruncate(t *testing.T, r MinimumRequirements, e interface{}) {
	t.Run(`Truncate`, func(t *testing.T) {
		TruncateSpec{Type: e, Subject: r}.Test(t)
	})
}

type TruncateTestEntity struct {
	ID string `ext:"ID"`
}
