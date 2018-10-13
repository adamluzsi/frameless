package queries_test

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

type testable func(t *testing.T, resource frameless.ExternalResource, reset func())

func (fn testable) Test(t *testing.T, resource frameless.ExternalResource, reset func()) {
	fn(t, resource, reset)
}
