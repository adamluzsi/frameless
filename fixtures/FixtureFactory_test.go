package fixtures_test

import (
	"github.com/adamluzsi/frameless/contracts"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactorySpec{
		Type:           GenericFixtureFactoryExampleType{},
		FixtureFactory: fixtures.FixtureFactory{},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}
