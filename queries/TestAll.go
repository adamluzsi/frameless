package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestAll(t *testing.T, e frameless.Resource, reset func()) {
	TestExportedEntity(t, e, reset)
	TestUnexportedEntity(t, e, reset)
	TestNotImplementedQuery(t, e, reset)
}
