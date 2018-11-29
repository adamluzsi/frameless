package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestAll(t *testing.T, r frameless.Resource) {
	TestExportedEntity(t, r)
	TestUnexportedEntity(t, r)
	TestNotImplementedQuery(t, r)
}
