package specs

import (
	"testing"
)

type MinimumRequirements interface {
	Save
	FindByID
	DeleteByID
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements) {
	t.Run("TestMinimumRequirements", func(t *testing.T) {

		shared := func(t *testing.T, entity interface {}) {
			SaveSpec{Entity: entity, Subject: r}.Test(t)
			FindByIDSpec{Type: entity, Subject: r}.Test(t)
			DeleteByIDSpec{Type: entity, Subject: r}.Test(t)
		}

		shared(t, ExportedEntity{})
		shared(t, unexportedEntity{})

	})
}
