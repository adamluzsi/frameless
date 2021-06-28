package fixtures

import (
	"github.com/adamluzsi/testcase/fixtures"
)

var Factory = &fixtures.Factory{
	Random:  Random,
	Options: []fixtures.Option{fixtures.SkipByTag(`ext`, "id", "ID")},
}
