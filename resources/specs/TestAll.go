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

func TestAll(t *testing.T, r Resource, e interface{}, f FixtureFactory) {
	t.Run(`specs`, func(t *testing.T) {

		t.Run(`CREATE`, func(t *testing.T) {
			TestSave(t, r, e, f)
		})

		t.Run(`READ`, func(t *testing.T) {
			TestFindAll(t, r, e, f)
			TestFindByID(t, r, e, f)
		})

		t.Run(`UPDATE`, func(t *testing.T) {
			TestUpdate(t, r, e, f)
		})

		t.Run(`DELETE`, func(t *testing.T) {
			TestDelete(t, r, e, f)
			TestDeleteByID(t, r, e, f)
			TestTruncate(t, r, e, f)
			TestPurge(t, r, e, f)
		})

	})
}
