package crudcontracts

import (
	"fmt"
	"testing"

	crudtest "go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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

func createDummyID[Entity, ID any](t *testcase.T, subject crd[Entity, ID], config Config[Entity, ID]) ID {
	ent := config.MakeEntity(t)
	ctx := config.MakeContext()
	crudtest.Create[Entity, ID](t, subject, ctx, &ent)
	id := crudtest.HasID[Entity, ID](t, ent)
	crudtest.Delete[Entity, ID](t, subject, ctx, &ent)
	return id
}

type TestingTBContextKey struct{}
