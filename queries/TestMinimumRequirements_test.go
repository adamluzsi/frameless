package queries_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries"
	"testing"
)

// TestAll test is production... just joking :)
func TestMinimumRequirements(t *testing.T) {
	var _ frameless.Query = testable(queries.TestMinimumRequirements)
}
