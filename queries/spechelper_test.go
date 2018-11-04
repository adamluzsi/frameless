package queries_test

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

type testable func(t *testing.T, resource frameless.Resource, reset func())

func (fn testable) Test(t *testing.T, resource frameless.Resource, reset func()) {
	fn(t, resource, reset)
}
