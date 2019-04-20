package specs

import (
	"github.com/adamluzsi/frameless/fixtures"
)

func newFixture(entity interface {}) interface {} {

	newEntity := fixtures.New(entity)

	if _, ok := LookupID(newEntity); ok {
		SetID(newEntity, "")
	}

	return newEntity

}


type entityWithoutIDField struct {
	Data string
}
