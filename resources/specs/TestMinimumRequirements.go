package specs

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
)

type MinimumRequirements interface {
	resources.Save
	resources.FindByID
	resources.DeleteByID
	resources.Truncate
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}, fixture FixtureFactory) {
	TestSave(t, r, TypeAsStruct, fixture)
	TestFindByID(t, r, TypeAsStruct, fixture)
	TestDeleteByID(t, r, TypeAsStruct, fixture)
	TestTruncate(t, r, TypeAsStruct, fixture)
}
