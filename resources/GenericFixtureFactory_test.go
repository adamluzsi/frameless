package resources_test

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

func TestGenericFixtureFactory(t *testing.T) {
	resources.FixtureFactorySpec{
		Type:           GenericFixtureFactoryExampleType{},
		FixtureFactory: resources.GenericFixtureFactory{},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:ID`
	STR string
	BDY string
}
