package frameless_test

import (
	"testing"

	"github.com/adamluzsi/frameless"
)

// I had to add this here because the godoc was removing ful example of the query use case
func (quc InactiveUsers) Test(suite *testing.T, storage frameless.Storage, cleanup func()) {
	quc.TTest(suite, storage, cleanup)
}
