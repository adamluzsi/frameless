package storages

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/specs"
)

type Resource interface {
	resources.Saver
	resources.Finder
	resources.FinderAll
	resources.Updater
	resources.Deleter
	resources.Truncater
}

func TestCommonSpec(t *testing.T, r Resource) {
	CommonSpec{Subject: r}.Test(t)
}

type CommonSpec struct {
	Subject        Resource
	FixtureFactory specs.GenericFixtureFactory
}

func (spec CommonSpec) Test(t *testing.T) {
	specs.TestAll(t, spec.Subject, ExportedEntity{}, spec.FixtureFactory)
	specs.TestAll(t, spec.Subject, unexportedEntity{}, spec.FixtureFactory)
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
