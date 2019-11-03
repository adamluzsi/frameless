package specs_test

import (
	"testing"

	"github.com/adamluzsi/frameless/resources/specs"
)

func TestGenericFixtureFactory(t *testing.T) {
	specs.FixtureFactorySpec{
		Type:           GenericFixtureFactoryExampleType{},
		FixtureFactory: specs.GenericFixtureFactory{},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:ID`
	STR string
	BDY string
}
