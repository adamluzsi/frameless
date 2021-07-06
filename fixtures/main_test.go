package fixtures_test

import (
	"testing"

	"github.com/adamluzsi/frameless/contracts"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactoryContract{
		T: GenericFixtureFactoryExampleType{},
		FixtureFactory: func(tb testing.TB) contracts.FixtureFactory {
			return fixtures.NewFactory()
		},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}
