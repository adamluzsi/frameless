package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestAll(t *testing.T, e frameless.ExternalResource, r func()) {
	TestExportedEntity(t, e, r)
	TestUnexportedEntity(t, e, r)
	TestNotImplementedQuery(t, e, r)
}
