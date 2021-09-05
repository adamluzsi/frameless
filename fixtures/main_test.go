package fixtures_test

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactory{
		T: GenericFixtureFactoryExampleType{},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}
