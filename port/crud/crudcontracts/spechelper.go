package crudcontracts

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud"
	crudtest "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type Contract interface {
	testcase.Suite
	testcase.OpenSuite
}

func getID[ENT, ID any](tb testing.TB, c Config[ENT, ID], ent ENT) ID {
	id, ok := c.IDA.Lookup(ent)
	assert.Must(tb).True(ok,
		`id was expected to be present for the entity`,
		assert.Message(fmt.Sprintf(` (%#v)`, ent)))
	return id
}

func createDummyID[ENT, ID any](t *testcase.T, subject crd[ENT, ID], config Config[ENT, ID]) ID {
	ent := config.MakeEntity(t)
	ctx := config.MakeContext()
	crudtest.Create[ENT, ID](t, subject, ctx, &ent)
	id := crudtest.HasID[ENT, ID](t, &ent)
	crudtest.Delete[ENT, ID](t, subject, ctx, &ent)
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
		crudtest.HasID[ENT, ID](tb, &ent)
		return ent
	}
	tb.Skip("test can't continue due to unable work with an entity present in the resource")
	return *new(ENT)
}

func makeEntity[ENT, ID any](tb testing.TB, FailNow func(), c Config[ENT, ID], subject any, mk func(testing.TB) ENT, mkFuncName string) ENT {
	tb.Helper()
	assert.NotNil(tb, mk)
	ent := mk(tb)
	assert.NotEmpty(tb, ent)
	if id, ok := lookupID[ID](c, ent); ok {
		if finder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
			_, found, err := finder.FindByID(c.MakeContext(), id)
			if err == nil && found {
				return ent
			}
		}
	}
	if creator, ok := subject.(crud.Creator[ENT]); ok {
		crudtest.Create[ENT, ID](tb, creator, c.MakeContext(), &ent)
		return ent
	}
	tb.Log("unable to ensure that the test has an entity that will be included in the query results")
	tb.Log("either ensure that the entity making function persist the entity in the subject")
	tb.Logf("or make sure that %T implements crud.Creator", subject)
	tb.Logf("(%s)", mkFuncName)
	FailNow()
	return *new(ENT)
}

func lookupID[ID, ENT any](c Config[ENT, ID], ent ENT) (ID, bool) {
	return c.IDA.Lookup(ent)
}

func setID[ENT, ID any](tb testing.TB, c Config[ENT, ID], ptr *ENT, id ID) {
	assert.NoError(tb, c.IDA.Set(ptr, id))
}

func tryDelete[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, v ENT) {
	id, ok := c.IDA.Lookup(v)
	if !ok {
		return
	}
	deleter, ok := resource.(crud.ByIDDeleter[ID])
	if !ok {
		return
	}
	if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
		_, found, err := finder.FindByID(ctx, id)
		assert.NoError(tb, err)
		if !found {
			return
		}
	}
	err := deleter.DeleteByID(ctx, id)
	if errors.Is(err, crud.ErrNotFound) {
		return
	}
	assert.NoError(tb, err)
}

func changeENT[ENT, ID any](tb testing.TB, c Config[ENT, ID], ptr *ENT) {
	assert.NotNil(tb, ptr)
	if c.ChangeEntity != nil {
		c.ChangeEntity(tb, ptr)
		return
	}
	id, _ := lookupID[ID](c, *ptr)
	*ptr = random.Unique(func() ENT { return c.MakeEntity(tb) }, *ptr)
	setID(tb, c, ptr, id)
}

func shouldPresent[ENT, ID any](t *testcase.T, c Config[ENT, ID], resource any, ctx context.Context, id ID) *ENT {
	t.Helper()
	return crudtest.IsPresent(t, shouldByIDFinder[ENT, ID](t, resource), ctx, id)
}

func shouldAbsent[ENT, ID any](t *testcase.T, c Config[ENT, ID], resource any, ctx context.Context, id ID) {
	t.Helper()
	crudtest.IsAbsent(t, shouldByIDFinder[ENT, ID](t, resource), ctx, id)
}

func shouldByIDFinder[ENT, ID any](tb testing.TB, resource any) crud.ByIDFinder[ENT, ID] {
	tb.Helper()
	bif, ok := resource.(crud.ByIDFinder[ENT, ID])
	if !ok {
		tb.Skipf("test must be skipped, as assertion requires %s to be implemented by %T", reflectkit.TypeOf[crud.ByIDFinder[ENT, ID]]().String(), resource)
	}
	return bif
}

func shouldFindByID[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, id ID) (ENT, bool, error) {
	tb.Helper()
	return shouldByIDFinder[ENT, ID](tb, resource).FindByID(ctx, id)
}

func shouldCreate[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, ptr *ENT) {
	tb.Helper()
	if subject, ok := resource.(crud.Creator[ENT]); ok {
		crudtest.Create[ENT, ID](tb, subject, ctx, ptr)
		return
	}
	if subject, ok := resource.(crud.Saver[ENT]); ok {
		crudtest.Save[ENT, ID](tb, subject, ctx, ptr)
		return
	}
	tb.Skipf("unable to continue with this testing scenario, as %T doesn't implement neither crud.Creator or crud.Saver", resource)
}

func shouldDelete[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, v ENT) {
	tb.Helper()
	id, ok := c.IDA.Lookup(v)
	if !ok {
		return
	}
	deleter, ok := resource.(crud.ByIDDeleter[ID])
	if !ok {
		tb.Skipf("unable to execute testing scenario, %s is not implemented", reflectkit.TypeOf[crud.ByIDDeleter[ID]]().String())
	}
	if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
		_, found, err := finder.FindByID(ctx, id)
		assert.NoError(tb, err)
		if !found {
			return
		}
	}
	err := deleter.DeleteByID(ctx, id)
	if errors.Is(err, crud.ErrNotFound) {
		return
	}
	assert.NoError(tb, err)
}

func testingRunFlagProvided() bool {
	runFlag := flag.Lookup("test.run")
	return runFlag != nil && runFlag.Value.String() != ""
}

func tryClose(c io.Closer) {
	if c == nil {
		return
	}
	_ = c.Close()
}

func shouldIterEventuallyError[ENT any](tb testing.TB, fn crud.QueryManyClosure[ENT]) (rErr error) {
	iter, err := fn()
	assert.AnyOf(tb, func(a *assert.A) {
		a.Case(func(t assert.It) {
			t.Must.Error(err)
			rErr = err
		})
		if iter != nil {
			a.Case(func(t assert.It) {
				_, err := iterators.Collect(iter)
				t.Must.Error(err)
				rErr = err
			})
		}
	})
	return
}
