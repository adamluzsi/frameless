package queries_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries"
	"testing"
)

// Test test is production... just joking :)
func TestTest(t *testing.T) {
	var _ frameless.Query = testable(queries.Test)
}

type testable func(t *testing.T, resource frameless.ExternalResource, reset func())

func (fn testable) Test(t *testing.T, resource frameless.ExternalResource, reset func()) {
	fn(t, resource, reset)
}
