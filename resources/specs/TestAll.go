package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type resource interface {
	resources.Saver
	resources.FinderByID
	resources.FinderAll
	resources.Updater
	resources.Deleter
	resources.DeleterByID
	resources.Truncater
}

func TestAll(t *testing.T, r resource, e interface{}, f FixtureFactory) {
	t.Run(`CREATE`, func(t *testing.T) {
		TestSaver(t, r, e, f)
	})

	t.Run(`READ`, func(t *testing.T) {
		TestFinderAll(t, r, e, f)
		TestFinderByID(t, r, e, f)
	})

	t.Run(`UPDATE`, func(t *testing.T) {
		TestUpdater(t, r, e, f)
	})

	t.Run(`DELETE`, func(t *testing.T) {
		TestDeleter(t, r, e, f)
		TestDeleterByID(t, r, e, f)
		TestTruncater(t, r, e, f)
	})
}
