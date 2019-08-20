package specs

import (
	"testing"
)

func TestAll(t *testing.T, r resource, e interface{}, f FixtureFactory) {
	t.Run(`CREATE`, func(t *testing.T) {
		SaverSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})

	t.Run(`READ`, func(t *testing.T) {
		FinderSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
		FinderAllSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})

	t.Run(`UPDATE`, func(t *testing.T) {
		UpdaterSpec{EntityType: e, FixtureFactory: f, Subject: r}.Test(t)
	})

	t.Run(`DELETE`, func(t *testing.T) {
		DeleterSpec{Subject: r, EntityType: e, FixtureFactory: f}.Test(t)
		TruncaterSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	})
}
