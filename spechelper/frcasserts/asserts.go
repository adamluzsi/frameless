package frcasserts

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	sh "github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/testcase/assert"
)

func HasID[Entity, ID any](tb testing.TB, ptr *Entity) (id ID) {
	tb.Helper()
	Eventually.Assert(tb, func(it assert.It) {
		var ok bool
		id, ok = extid.Lookup[ID](ptr)
		it.Must.True(ok)
		it.Must.NotEmpty(id)
	})
	return
}

func IsFindable[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, id ID) *Entity {
	tb.Helper()
	var ent Entity
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(Entity), id)
	Eventually.Assert(tb, func(it assert.It) {
		e, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.True(found, errMessage)
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
		it.Must.False(found, errMessage)
	})
}

func HasEntity[Entity, ID any](tb testing.TB, subject crud.ByIDFinder[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()
	id := HasID[Entity, ID](tb, ptr)
	Eventually.Assert(tb, func(it assert.It) {
		// IsFindable yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		it.Must.Equal(ptr, IsFindable(it, subject, ctx, id))
	})
}

func Create[Entity, ID any](tb testing.TB, subject sh.CRD[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()

	assert.Must(tb).Nil(subject.Create(ctx, ptr))
	id := HasID[Entity, ID](tb, ptr)
	tb.Cleanup(func() {
		_, found, err := subject.FindByID(ctx, id)
		if err != nil || !found {
			return
		}
		_ = subject.DeleteByID(ctx, id)
	})
	IsFindable[Entity, ID](tb, subject, ctx, id)
	tb.Logf("given entity is created: %#v", ptr)
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
	IsFindable[Entity, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.Update(ctx, ptr))
	Eventually.Assert(tb, func(it assert.It) {
		entity := IsFindable[Entity, ID](it, subject, ctx, id)
		it.Must.Equal(ptr, entity)
	})
	tb.Logf(`entity is updated: %#v`, ptr)
}

func Delete[Entity, ID any](tb testing.TB, subject sh.CRD[Entity, ID], ctx context.Context, ptr *Entity) {
	tb.Helper()
	id := HasID[Entity, ID](tb, ptr)
	IsFindable[Entity, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.DeleteByID(ctx, id))
	IsAbsent[Entity, ID](tb, subject, ctx, id)
	tb.Logf("entity is deleted: %#v", ptr)
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
