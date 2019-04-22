package specs

import (
	"testing"
)

type Resource interface {
	Save
	FindByID
	FindAll
	Update
	Delete
	DeleteByID
	Truncate
	Purge
}

func TestAll(t *testing.T, r Resource, e interface{}) {
	t.Run(`specs`, func(t *testing.T) {

		t.Run(`CREATE`, func(t *testing.T) {
			TestSave(t, r, e)
		})

		t.Run(`READ`, func(t *testing.T) {
			TestFindAll(t, r, e)
			TestFindByID(t, r, e)
		})

		t.Run(`UPDATE`, func(t *testing.T) {
			TestUpdate(t, r, e)
		})

		t.Run(`DELETE`, func(t *testing.T) {
			TestDelete(t, r, e)
			TestDeleteByID(t, r, e)
			TestTruncate(t, r, e)
			TestPurge(t, r, e)
		})

	})
}

func TestAllWithExampleEntities(t *testing.T, r Resource) {
	TestAll(t, r, ExportedEntity{})
	TestAll(t, r, unexportedEntity{})
}
