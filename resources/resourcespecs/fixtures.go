package resourcespecs

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
)

func newFixture(entity frameless.Entity) frameless.Entity {

	newEntity := fixtures.New(entity)

	if _, ok := LookupID(newEntity); ok {
		SetID(newEntity, "")
	}

	return newEntity

}
