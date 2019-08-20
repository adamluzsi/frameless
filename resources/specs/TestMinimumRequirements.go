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

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}, fixture FixtureFactory) {
	TestSaver(t, r, TypeAsStruct, fixture)
	TestFinder(t, r, TypeAsStruct, fixture)
	TestDeleter(t, r, TypeAsStruct, fixture)
	TestTruncater(t, r, TypeAsStruct, fixture)
}
