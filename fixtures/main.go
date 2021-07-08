package fixtures

import (
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func NewFactory(tb testing.TB) *fixtures.Factory {
	return &fixtures.Factory{
		Random:  getRandom(tb),
		Options: []fixtures.Option{fixtures.SkipByTag(`ext`, "id", "ID")},
	}
}

var Random = fixtures.Random

func getRandom(tb testing.TB) *random.Random {
	if tcTB, ok := tb.(*testcase.T); ok {
		return tcTB.Random
	}
	return Random
}
