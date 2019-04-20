package specs

import (
	"testing"
)

type resource interface {
	Save
	Update
	Delete
	FindAll
	FindByID
	DeleteByID
}

func TestAll(t *testing.T, r resource) {
	TestMinimumRequirements(t, r)
	TestExportedEntity(t, r)
	TestUnexportedEntity(t, r)
}
