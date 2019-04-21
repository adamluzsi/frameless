package specs

import (
	"testing"
)

type Resource interface {
	MinimumRequirements

	Purge
	Update
	Delete
	FindAll
}

func TestAll(t *testing.T, r Resource) {
	t.Run(`specs`, func(t *testing.T) {
		PurgeSpec{Subject: r}.Test(t)
		TestMinimumRequirements(t, r)
		TestExportedEntity(t, r)
		TestUnexportedEntity(t, r)
	})
}
