package crudtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/pointer"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	sh "go.llib.dev/frameless/spechelper"

	"go.llib.dev/testcase/assert"
)

var Waiter = assert.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

var Eventually = assert.Retry{
	Strategy: &Waiter,
}

func HasID[Entity, ID any](tb testing.TB, ent Entity) (id ID) {
	tb.Helper()
	// TODO: remove this, makes no sense to wait for an async unsafe id value setting.
	//       It feels like supporting bad implementation designs.
	Eventually.Assert(tb, func(it assert.It) {
		var ok bool
		id, ok = extid.Lookup[ID](ent)
		it.Must.True(ok)
		it.Must.NotEmpty(id)
	})
	return
}

// IsFindable
//
// DEPRECATED: use the new name: IsPresent
func IsFindable[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, id ID) *Entity {
	tb.Helper()
	return IsPresent[Entity, ID](tb, subject, ctx, id)
}

func IsPresent[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, id ID) *Entity {
	tb.Helper()
	var ent Entity
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(Entity), id)
	Eventually.Assert(tb, func(it assert.It) {
		e, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.True(found, assert.Message(errMessage))
		ent = e
	})
	return &ent
}

func IsAbsent[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, id ID) {
	tb.Helper()
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be absent", *new(Entity), id)
	Eventually.Assert(tb, func(it assert.It) {
		_, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.False(found, assert.Message(errMessage))
	})
}

func HasEntity[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()
	id := HasID[Entity, ID](tb, pointer.Deref(ptr))
	Eventually.Assert(tb, func(it assert.It) {
		// IsFindable yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		it.Must.Equal(ptr, IsPresent(it, subject, ctx, id))
	})
}

func Create[Entity, ID any](tb testing.TB, subject crud.Creator[Entity], ctx context.Context, ptr *Entity) {
	tb.Helper()
	assert.Must(tb).Nil(subject.Create(ctx, ptr))
	id := HasID[Entity, ID](tb, pointer.Deref(ptr))
	tb.Cleanup(func() {
		del, ok := subject.(crud.ByIDDeleter[ID])
		if !ok {
			tb.Logf("skipping cleanup as %T doesn't implement crud.ByIDDeleter", subject)
			tb.Logf("make sure to manually clean up %T#%v", *new(Entity), id)
			return
		}
		_ = del.DeleteByID(ctx, id)
	})
	if finder, ok := subject.(crud.ByIDFinder[Entity, ID]); ok {
		IsPresent[Entity, ID](tb, finder, ctx, id)
	}
}

type updater[Entity, ID any] interface {
	crud.Updater[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}

func Update[Entity, ID any](tb testing.TB, subject updater[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()
	id, _ := extid.Lookup[ID](ptr)
	// IsFindable ensures that by the time Update is executed,
	// the entity is present in the resource.
	IsPresent[Entity, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.Update(ctx, ptr))
	Eventually.Assert(tb, func(it assert.It) {
		entity := IsPresent[Entity, ID](it, subject, ctx, id)
		it.Must.Equal(ptr, entity)
	})
}

func Delete[Entity, ID any](tb testing.TB, subject sh.CRD[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()
	id := HasID[Entity, ID](tb, pointer.Deref(ptr))
	IsPresent[Entity, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.DeleteByID(ctx, id))
	IsAbsent[Entity, ID](tb, subject, ctx, id)
}

type deleteAllDeleter[Entity, ID any] interface {
	crud.AllFinder[Entity]
	crud.AllDeleter
}

func DeleteAll[Entity, ID any](tb testing.TB, subject deleteAllDeleter[Entity, ID], ctx context.Context) {
	tb.Helper()
	assert.Must(tb).Nil(subject.DeleteAll(ctx))
	Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	Eventually.Assert(tb, func(it assert.It) {
		count, err := iterators.Count(subject.FindAll(ctx))
		it.Must.Nil(err)
		it.Should.True(count == 0, `no entity was expected to be found`)
	})
}

func CountIs[T any](tb testing.TB, iter iterators.Iterator[T], expected int) {
	tb.Helper()
	count, err := iterators.Count(iter)
	assert.Must(tb).Nil(err)
	assert.Must(tb).Equal(expected, count)
}
