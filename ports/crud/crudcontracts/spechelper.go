package crudcontracts

import (
	"fmt"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"testing"
)

type Contract interface {
	testcase.Suite
	testcase.OpenSuite
}

func getID[Ent, ID any](tb testing.TB, ent Ent) ID {
	id, ok := extid.Lookup[ID, Ent](ent)
	assert.Must(tb).True(ok,
		`id was expected to be present for the entity`,
		assert.Message(fmt.Sprintf(` (%#v)`, ent)))
	return id
}
