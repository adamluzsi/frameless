package fixtures

import (
	"context"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/adamluzsi/testcase/random"
)

func NewFactory(tb testing.TB) *Factory {
	return &Factory{
		Factory: &fixtures.Factory{
			Random:  getRandom(tb),
			Options: []fixtures.Option{fixtures.SkipByTag(`ext`, "id", "ID")},
		},
	}
}

var Random = fixtures.Random

func getRandom(tb testing.TB) *random.Random {
	if tcTB, ok := tb.(*testcase.T); ok {
		return tcTB.Random
	}
	return Random
}

type Factory struct{ *fixtures.Factory }

func (f *Factory) Fixture(T interface{}, ctx context.Context) (_T interface{}) {
	return f.Factory.Create(T)
}
