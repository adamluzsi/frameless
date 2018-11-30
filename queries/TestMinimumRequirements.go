package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestMinimumRequirements(t *testing.T, r frameless.Resource) {

	t.Run("TestMinimumRequirements", func(t *testing.T) {

		shared := func(t *testing.T, entity frameless.Entity) {
			SaveEntity{Entity: entity}.Test(t, r)
			FindByID{Type: entity}.Test(t, r)
			DeleteByID{Type: entity}.Test(t, r)
		}

		shared(t, ExportedEntity{})
		shared(t, unexportedEntity{})

	})
}
