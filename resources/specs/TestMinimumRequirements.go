package specs

import (
	"testing"
)

type MinimumRequirements interface {
	Save
	FindByID
	DeleteByID
	Truncate
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}, fixture FixtureFactory) {
	TestSave(t, r, TypeAsStruct, fixture)
	TestFindByID(t, r, TypeAsStruct, fixture)
	TestDeleteByID(t, r, TypeAsStruct, fixture)
	TestTruncate(t, r, TypeAsStruct, fixture)
}
