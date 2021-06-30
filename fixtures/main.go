package fixtures

import (
	"github.com/adamluzsi/testcase/fixtures"
)

var Random = fixtures.Random
var SecureRandom = fixtures.SecureRandom

var Factory interface{}

func NewFactory() *fixtures.Factory {
	return &fixtures.Factory{
		Random:  Random,
		Options: []fixtures.Option{fixtures.SkipByTag(`ext`, "id", "ID")},
	}
}
