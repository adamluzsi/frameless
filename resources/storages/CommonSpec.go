package storages

import (
	"github.com/adamluzsi/frameless/resources"
	"testing"
)

type Resource interface {
	resources.Save
	resources.FindByID
	resources.FindAll
	resources.Update
	resources.Delete
	resources.DeleteByID
	resources.Truncate
}

func TestCommonSpec(t *testing.T, r Resource) {
	CommonSpec{Subject: r}.Test(t)
}

type CommonSpec struct {
	Subject        Resource
	FixtureFactory resources.GenericFixtureFactory
}

func (spec CommonSpec) Test(t *testing.T) {
	resources.TestAll(t, spec.Subject, ExportedEntity{}, spec.FixtureFactory)
	resources.TestAll(t, spec.Subject, unexportedEntity{}, spec.FixtureFactory)
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
