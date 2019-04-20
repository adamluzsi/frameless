package specs

import (
	"testing"
)

type Resource interface {
	Purge
	Save
	Update
	Delete
	FindAll
	FindByID
	DeleteByID
}

func TestAll(t *testing.T, r Resource) {
	t.Run(`specs`, func(t *testing.T) {
		TestMinimumRequirements(t, r)
		TestExportedEntity(t, r)
		TestUnexportedEntity(t, r)
	})
}
