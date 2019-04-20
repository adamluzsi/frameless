package queries

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

func TestAll(t *testing.T, r resources.Resource) {
	t.Run(`queries`, func(t *testing.T) {
		TestMinimumRequirements(t, r)
		TestNotImplementedQuery(t, r)
		TestExportedEntity(t, r)
		TestUnexportedEntity(t, r)
	})
}
