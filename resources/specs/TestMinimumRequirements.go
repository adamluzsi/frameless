package specs

import (
	"fmt"
	"github.com/adamluzsi/frameless/reflects"
	"testing"
)

type MinimumRequirements interface {
	Save
	FindByID
	DeleteByID
}

func TestMinimumRequirementsWithExampleEntities(t *testing.T, r MinimumRequirements) {
	t.Run(`Minimum Requirements`, func(t *testing.T) {
		TestMinimumRequirementsWith(t, r, ExportedEntity{})
		TestMinimumRequirementsWith(t, r, unexportedEntity{})
	})
}

func TestMinimumRequirementsWith(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}) {
	qualifiedName := reflects.FullyQualifiedName(TypeAsStruct)
	testRunName := fmt.Sprintf(`Test Minimum Requirements For %s`, qualifiedName)

	t.Run(testRunName, func(t *testing.T) {
		SaveSpec{Entity: TypeAsStruct, Subject: r}.Test(t)
		FindByIDSpec{Type: TypeAsStruct, Subject: r}.Test(t)
		DeleteByIDSpec{Type: TypeAsStruct, Subject: r}.Test(t)
	})
}
