package frameless_test

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

// I had to add this here because the godoc was removing ful example of the query use case
func (quc InactiveUsers) Test(suite *testing.T, resource resources.Resource) {
	quc.TTest(suite, resource)
}
