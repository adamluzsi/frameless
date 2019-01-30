package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

func TestMinimumRequirements(t *testing.T, r resources.Resource) {
	t.Run("TestMinimumRequirements", func(t *testing.T) {

		shared := func(t *testing.T, entity frameless.Entity) {
			Save{Entity: entity}.Test(t, r)
			FindByID{Type: entity}.Test(t, r)
			DeleteByID{Type: entity}.Test(t, r)
		}

		shared(t, ExportedEntity{})
		shared(t, unexportedEntity{})

	})
}
