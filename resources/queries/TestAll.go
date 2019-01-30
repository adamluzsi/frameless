package queries

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

func TestAll(t *testing.T, r resources.Resource) {
	TestMinimumRequirements(t, r)
	TestNotImplementedQuery(t, r)
	TestExportedEntity(t, r)
	TestUnexportedEntity(t, r)
}
