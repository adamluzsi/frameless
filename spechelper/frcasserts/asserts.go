package frcasserts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	sh "github.com/adamluzsi/frameless/spechelper"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func HasID[Ent, ID any](tb testing.TB, ptr *Ent) (id ID) {
	tb.Helper()
	Eventually.Assert(tb, func(it assert.It) {
		var ok bool
		id, ok = extid.Lookup[ID](ptr)
		it.Must.True(ok)
		it.Must.NotEmpty(id)
	})
	return
}

func IsFindable[Ent, ID any](tb testing.TB, subject crud.ByIDFinder[Ent, ID], ctx context.Context, id ID) *Ent {
	tb.Helper()
	var ent Ent
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(Ent), id)
	Eventually.Assert(tb, func(it assert.It) {
		e, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.True(found, errMessage)
		ent = e
	})
	return &ent
}

func IsAbsent[Ent, ID any](tb testing.TB, subject crud.ByIDFinder[Ent, ID], ctx context.Context, id ID) {
	tb.Helper()
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be absent", *new(Ent), id)
	Eventually.Assert(tb, func(it assert.It) {
		_, found, err := subject.FindByID(ctx, id)
		it.Must.Nil(err)
		it.Must.False(found, errMessage)
	})
}

func HasEntity[Ent, ID any](tb testing.TB, subject crud.ByIDFinder[Ent, ID], ctx context.Context, ptr *Ent) {
	tb.Helper()
	id := HasID[Ent, ID](tb, ptr)
	Eventually.Assert(tb, func(it assert.It) {
		// IsFindable yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		it.Must.Equal(ptr, IsFindable(it, subject, ctx, id))
	})
}

func Create[Ent, ID any](tb testing.TB, subject sh.CRD[Ent, ID], ctx context.Context, ptr *Ent) {
	tb.Helper()

	assert.Must(tb).Nil(subject.Create(ctx, ptr))
	id := HasID[Ent, ID](tb, ptr)
	tb.Cleanup(func() {
		_, found, err := subject.FindByID(ctx, id)
		if err != nil || !found {
			return
		}
		_ = subject.DeleteByID(ctx, id)
	})
	IsFindable[Ent, ID](tb, subject, ctx, id)
	tb.Logf("given entity is created: %#v", ptr)
}

type updater[Ent, ID any] interface {
	crud.Updater[Ent]
	crud.ByIDFinder[Ent, ID]
	crud.ByIDDeleter[ID]
}

func Update[Ent, ID any](tb testing.TB, subject updater[Ent, ID], ctx context.Context, ptr *Ent) {
	tb.Helper()
	id, _ := extid.Lookup[ID](ptr)
	// IsFindable ensures that by the time Update is executed,
	// the entity is present in the resource.
	IsFindable[Ent, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.Update(ctx, ptr))
	Eventually.Assert(tb, func(it assert.It) {
		entity := IsFindable[Ent, ID](it, subject, ctx, id)
		it.Must.Equal(ptr, entity)
	})
	tb.Logf(`entity is updated: %#v`, ptr)
}

func Delete[Ent, ID any](tb testing.TB, subject sh.CRD[Ent, ID], ctx context.Context, ptr *Ent) {
	tb.Helper()
	id := HasID[Ent, ID](tb, ptr)
	IsFindable[Ent, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.DeleteByID(ctx, id))
	IsAbsent[Ent, ID](tb, subject, ctx, id)
	tb.Logf("entity is deleted: %#v", ptr)
}

type deleteAllDeleter[Ent, ID any] interface {
	crud.AllFinder[Ent, ID]
	crud.AllDeleter
}

func DeleteAll[Ent, ID any](tb testing.TB, subject deleteAllDeleter[Ent, ID], ctx context.Context) {
	tb.Helper()
	assert.Must(tb).Nil(subject.DeleteAll(ctx))
	Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	Eventually.Assert(tb, func(it assert.It) {
		count, err := iterators2.Count(subject.FindAll(ctx))
		it.Must.Nil(err)
		it.Should.True(count == 0, `no entity was expected to be found`)
	})
}

func CountIs[T any](tb testing.TB, iter iterators2.Iterator[T], expected int) {
	tb.Helper()
	count, err := iterators2.Count(iter)
	assert.Must(tb).Nil(err)
	assert.Must(tb).Equal(expected, count)
}
