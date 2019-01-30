package queries_test

import (
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/queries"
	"testing"
)

// TestAll test is production... just joking :)
func TestTestAll(t *testing.T) {
	var _ resources.Query = testable(queries.TestAll)
}
