package fixtures_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactory{
		T: GenericFixtureFactoryExampleType{},
		Subject: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}
