package fixtures_test

import (
	"testing"

	"github.com/adamluzsi/frameless/contracts"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactorySpec{
		Type:           GenericFixtureFactoryExampleType{},
		FixtureFactory: fixtures.Factory,
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}
