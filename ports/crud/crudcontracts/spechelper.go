package crudcontracts

import (
	"fmt"
	"reflect"
	"testing"

	"go.llib.dev/frameless/ports/crud"
	crudtest "go.llib.dev/frameless/ports/crud/crudtest"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/spechelper"
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

type crd[ENT, ID any] interface {
	crud.Creator[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.ByIDDeleter[ID]
}

func isUnique[ENT any](ent ENT, oths ...ENT) bool {
	var unique bool = true
	for _, oth := range oths {
		if reflect.DeepEqual(ent, oth) {
			unique = false
			break
		}
	}
	return unique
}

func makeUnique[ENT any](tb testing.TB, mk func(tb testing.TB) ENT, oths ...ENT) (ENT, bool) {
	tb.Helper()
	var (
		ent ENT
		ok  bool
	)
	crudtest.Eventually.Strategy.While(func() bool {
		ent = mk(tb)
		ok = isUnique(ent, oths...)
		return !ok
	})
	return ent, ok
}

func ensureExistingEntity[ENT, ID any](tb testing.TB, c Config[ENT, ID], subject any, oths ...ENT) ENT {
	tb.Helper()
	if res, ok := subject.(spechelper.CRD[ENT, ID]); ok {
		ent, ok := makeUnique(tb, func(tb testing.TB) ENT {
			ent := c.MakeEntity(tb)
			crudtest.Create[ENT, ID](tb, res, c.MakeContext(), &ent)
			return ent
		}, oths...)
		if !ok {
			tb.Skip("was unable to create a unique value with MakeEntity + resource.Create, test can't continue")
		}
		return ent
	}
	if c.ExampleEntity != nil {
		ent, ok := makeUnique(tb, func(tb testing.TB) ENT {
			return c.ExampleEntity(tb)
		}, oths...)
		if !ok {
			tb.Skip("config ExampleEntity is not returning back a unique value, thus this test can't continue")
		}
		crudtest.HasID[ENT, ID](tb, ent)
		return ent
	}
	tb.Skip("test can't continue due to unable work with an entity present in the resource")
	return *new(ENT)
}

func makeEntity[ENT, ID any](tb testing.TB, c Config[ENT, ID], subject any, mk func(testing.TB) ENT) ENT {
	tb.Helper()
	assert.NotNil(tb, mk)
	ent := mk(tb)
	assert.NotEmpty(tb, ent)
	if _, ok := extid.Lookup[ID](ent); ok {
		return ent
	}
	if creator, ok := subject.(crud.Creator[ENT]); ok {
		crudtest.Create[ENT, ID](tb, creator, c.MakeContext(), &ent)
		return ent
	}
	tb.Log("unable to ensure that the test has an entity that will be included in the query results")
	tb.Log("either ensure that the entity making function persist the entity in the subject")
	tb.Logf("or make sure that %T implements crud.Creator", subject)
	tb.FailNow()
	return *new(ENT)
}
