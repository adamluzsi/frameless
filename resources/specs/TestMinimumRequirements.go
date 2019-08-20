package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type MinimumRequirements interface {
	resources.Saver
	resources.Finder
	resources.Deleter
	resources.Truncater
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, e interface{}, f FixtureFactory) {
	SaverSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	FinderSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
	Deleter{Subject: r, EntityType: e, FixtureFactory: f}.Test(t)
	TruncaterSpec{EntityType: e, Subject: r, FixtureFactory: f}.Test(t)
}
