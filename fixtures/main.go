package fixtures

import (
	"github.com/adamluzsi/testcase/fixtures"
)

var Random = fixtures.Random

func NewFactory() *fixtures.Factory {
	return &fixtures.Factory{
		Random:  Random,
		Options: []fixtures.Option{fixtures.SkipByTag(`ext`, "id", "ID")},
	}
}
