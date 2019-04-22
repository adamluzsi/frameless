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
	Truncate
}

func TestMinimumRequirements(t *testing.T, r MinimumRequirements, TypeAsStruct interface{}) {
	qualifiedName := reflects.FullyQualifiedName(TypeAsStruct)
	testRunName := fmt.Sprintf(`Test Minimum Requirements For %s`, qualifiedName)

	t.Run(testRunName, func(t *testing.T) {
		TestSave(t, r, TypeAsStruct)
		TestFindByID(t, r, TypeAsStruct)
		TestDeleteByID(t, r, TypeAsStruct)
		TestTruncate(t, r, TypeAsStruct)
	})
}
