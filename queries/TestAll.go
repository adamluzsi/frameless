package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestAll(t *testing.T, r frameless.Resource) {
	TestMinimumRequirements(t, r)
	TestNotImplementedQuery(t, r)
	TestExportedEntity(t, r)
	TestUnexportedEntity(t, r)
}
