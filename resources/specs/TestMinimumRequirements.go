package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type MinimumRequirements interface {
	resources.Saver
	resources.FinderByID
	resources.DeleterByID
	resources.Truncater
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}, fixture FixtureFactory) {
	TestSaver(t, r, TypeAsStruct, fixture)
	TestFinderByID(t, r, TypeAsStruct, fixture)
	TestDeleterByID(t, r, TypeAsStruct, fixture)
	TestTruncater(t, r, TypeAsStruct, fixture)
}
