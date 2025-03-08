package crudtest

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
)

var Waiter = assert.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

var Eventually = assert.Retry{
	Strategy: &Waiter,
}

func HasID[ENT, ID any](tb testing.TB, ent *ENT) (id ID) {
	tb.Helper()
	// TODO: remove this, makes no sense to wait for an async unsafe id value setting.
	//       It feels like supporting bad implementation designs.
	Eventually.Assert(tb, func(it assert.It) {
		var ok bool
		id, ok = extid.Lookup[ID](ent)
		assert.True(it, ok, assert.MessageF("expected to find external ID in %s", pp.Format(ent)))
		assert.NotEmpty(it, id)
	})
	return
}

func IsPresent[ENT, ID any](tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, id ID) *ENT {
	tb.Helper()
	var ent ENT
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(ENT), id)
	Eventually.Assert(tb, func(it assert.It) {
		e, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.True(found, assert.Message(errMessage))
		ent = e
	})
	return &ent
}

func IsAbsent[ENT, ID any](tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, id ID) {
	tb.Helper()
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be absent", *new(ENT), id)
	Eventually.Assert(tb, func(it assert.It) {
		_, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.False(found, assert.Message(errMessage))
	})
}

func HasEntity[ENT, ID any](tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := HasID[ENT, ID](tb, ptr)
	Eventually.Assert(tb, func(it assert.It) {
		// IsPresent yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		it.Must.Equal(ptr, IsPresent(it, subject, ctx, id))
	})
}

func Save[ENT, ID any](tb testing.TB, subject crud.Saver[ENT], ctx context.Context, ptr *ENT) {
	tb.Helper()
	assert.NoError(tb, subject.Save(ctx, ptr))
	cleanupENT[ENT, ID](tb, subject, ctx, ptr)
}

func Create[ENT, ID any](tb testing.TB, subject crud.Creator[ENT], ctx context.Context, ptr *ENT) {
	tb.Helper()
	assert.NoError(tb, subject.Create(ctx, ptr))
	cleanupENT[ENT, ID](tb, subject, ctx, ptr)
}

type updater[ENT, ID any] interface {
	crud.Updater[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.ByIDDeleter[ID]
}

func Update[ENT, ID any](tb testing.TB, subject updater[ENT, ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	id, _ := extid.Lookup[ID](ptr)
	// IsFindable ensures that by the time Update is executed,
	// the entity is present in the resource.
	IsPresent[ENT, ID](tb, subject, ctx, id)
	assert.Nil(tb, subject.Update(ctx, ptr))
	Eventually.Assert(tb, func(it assert.It) {
		entity := IsPresent[ENT, ID](it, subject, ctx, id)
		it.Must.Equal(ptr, entity)
	})
}

func Delete[ENT, ID any](tb testing.TB, subject crud.ByIDDeleter[ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := HasID[ENT, ID](tb, ptr)
	if finder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
		IsPresent[ENT, ID](tb, finder, ctx, id)
	}
	assert.Nil(tb, subject.DeleteByID(ctx, id))
	if finder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
		IsAbsent[ENT, ID](tb, finder, ctx, id)
	}
}

type deleteAllDeleter[ENT, ID any] interface {
	crud.AllFinder[ENT]
	crud.AllDeleter
}

func DeleteAll[ENT, ID any](tb testing.TB, subject deleteAllDeleter[ENT, ID], ctx context.Context) {
	tb.Helper()
	assert.Nil(tb, subject.DeleteAll(ctx))
	Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	Eventually.Assert(tb, func(t assert.It) {
		itr, err := subject.FindAll(ctx)
		assert.NoError(t, err)
		vs, err := iterkit.CollectErrIter(itr)
		assert.NoError(t, err)
		assert.Empty(t, vs, `no entity was expected to be found`)
	})
}

func CountIs[T any](tb testing.TB, iter iter.Seq[T], expected int) {
	tb.Helper()
	assert.Must(tb).Equal(expected, iterkit.Count(iter))
}

func cleanupENT[ENT, ID any](tb testing.TB, subject any, ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := HasID[ENT, ID](tb, ptr)
	tb.Cleanup(func() {
		del, ok := subject.(crud.ByIDDeleter[ID])
		if !ok {
			tb.Logf("skipping cleanup as %T doesn't implement crud.ByIDDeleter", subject)
			tb.Logf("make sure to manually clean up %T#%v", *new(ENT), id)
			return
		}
		_ = del.DeleteByID(ctx, id)
	})
	if finder, ok := subject.(crud.ByIDFinder[ENT, ID]); ok {
		IsPresent[ENT, ID](tb, finder, ctx, id)
	}
}
