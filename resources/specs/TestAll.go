package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type resource interface {
	resources.Save
	resources.FindByID
	resources.FindAll
	resources.Update
	resources.Delete
	resources.DeleteByID
	resources.Truncate
}

func TestAll(t *testing.T, r resource, e interface{}, f FixtureFactory) {
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
	})
}
