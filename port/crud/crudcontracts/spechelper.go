package crudcontracts

import (
	"context"
	"errors"
	"flag"
	"iter"
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

type Contract interface {
	testcase.Suite
	testcase.OpenSuite
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

// ensureExistingEntity will return with an entity that exists in the resource.
// Either with the mkFunc, or with c.MakeEntity or with c.ExampleEntity
func ensureExistingEntity[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, mkFunc func() ENT, oths ...ENT) ENT {
	tb.Helper()

	ent, ok := makeUnique(tb, func(tb testing.TB) ENT {
		tb.Helper()

		if mkFunc != nil {
			return mkFunc()
		}

		if c.ExampleEntity != nil {
			return c.ExampleEntity(tb)
		}

		if c.MakeEntity != nil {
			return c.MakeEntity(tb)
		}

		tb.Skipf("no make function to create a %s", reflectkit.TypeOf[ENT]().String())
		return *new(ENT)
	}, oths...)

	if !ok {
		tb.Skip("was unable to create a unique value with MakeEntity + resource.Create, test can't continue")
	}

	ctx := c.MakeContext(tb)

	if id, ok := lookupID[ID, ENT](c, ent); ok {

		if finder, canFindByID := resource.(crud.ByIDFinder[ENT, ID]); canFindByID {
			_, found, err := finder.FindByID(ctx, id)
			assert.NoError(tb, err)
			if found {
				return ent
			}
		}

	}

	shouldStore(tb, c, resource, &ent)

	return ent
}

func makeEntity[ENT, ID any](tb testing.TB, FailNow func(), c Config[ENT, ID], resource any, mk func() ENT, mkFuncName string) ENT {
	tb.Helper()
	assert.NotNil(tb, mk)
	ent := mk()
	assert.NotEmpty(tb, ent)
	if id, ok := lookupID[ID](c, ent); ok {
		if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
			_, found, err := finder.FindByID(c.MakeContext(tb), id)
			if err == nil && found {
				return ent
			}
		}
	}
	if creator, ok := resource.(crud.Creator[ENT]); ok {
		c.Helper().Create(tb, creator, c.MakeContext(tb), &ent)
		return ent
	}
	if saver, ok := resource.(crud.Saver[ENT]); ok {
		c.Helper().Save(tb, saver, c.MakeContext(tb), &ent)
		return ent
	}
	tb.Log("unable to ensure that the test has an entity that will be included in the query results")
	tb.Log("either ensure that the entity making function persist the entity in the subject")
	tb.Logf("or make sure that %T implements crud.Creator", resource)
	tb.Logf("(%s)", mkFuncName)
	FailNow()
	return *new(ENT)
}

func lookupID[ID, ENT any](c Config[ENT, ID], ent ENT) (ID, bool) {
	id, ok := c.IDA.Lookup(ent)
	if !ok && reflect.ValueOf(id).CanInt() {
		// int is an accepted zero value due to many system stores data under indexes, which are starting from zero.
		ok = true
	}
	return id, ok
}

func setID[ENT, ID any](tb testing.TB, c Config[ENT, ID], ptr *ENT, id ID) {
	assert.NoError(tb, c.IDA.Set(ptr, id))
}

func tryDelete[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, v ENT) {
	id, ok := lookupID(c, v)
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

func (c Config[ENT, ID]) ModifyEntity(tb testing.TB, ptr *ENT) {
	tb.Helper()
	assert.NotNil(tb, ptr, "crudcontracts.Config#ModifyEntity called with nil pointer")
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

func storer[ENT, ID any](c Config[ENT, ID], resource any) (func(tb testing.TB, ptr *ENT), bool) {
	if subject, ok := resource.(crud.Creator[ENT]); ok {
		return func(tb testing.TB, ptr *ENT) {
			tb.Helper()
			c.Helper().Create(tb, subject, c.MakeContext(tb), ptr)
		}, true
	}
	if subject, ok := resource.(crud.Saver[ENT]); ok {
		return func(tb testing.TB, ptr *ENT) {
			tb.Helper()
			c.Helper().Save(tb, subject, c.MakeContext(tb), ptr)
		}, true
	}
	return nil, false
}

func shouldStore[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ptr *ENT) {
	tb.Helper()
	if s, ok := storer[ENT, ID](c, resource); ok {
		s(tb, ptr)
		return
	}
	tb.Skipf("unable to continue with this testing scenario, as %T doesn't implement neither crud.Creator or crud.Saver", resource)
}

func shouldDelete[ENT, ID any](tb testing.TB, c Config[ENT, ID], resource any, ctx context.Context, v ENT) {
	tb.Helper()
	id, ok := lookupID(c, v)
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

func shouldIterEventuallyError[ENT any](tb testing.TB, fn func() iter.Seq2[ENT, error]) error {
	itr := fn()
	_, err := iterkit.CollectErr(itr)
	assert.Error(tb, err)
	return err
}
