package asserts

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/frameless/iterators"
)

type resource[T any, ID any] interface {
	frameless.Creator[T]
	frameless.Finder[T, ID]
	frameless.Deleter[ID]
}

func HasID[T any, ID any](tb testing.TB, ptr *T) (id ID) {
	tb.Helper()
	Eventually.Assert(tb, func(it assert.It) {
		var ok bool
		id, ok = extid.Lookup[ID](ptr)
		it.Must.True(ok)
		it.Must.NotEmpty(id)
	})
	return
}

func IsFindable[T any, ID any](tb testing.TB, subject frameless.Finder[T, ID], ctx context.Context, id ID) *T {
	tb.Helper()
	var ptr *T
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(T), id)
	Eventually.Assert(tb, func(it assert.It) {
		ptr = new(T)
		found, err := subject.FindByID(ctx, ptr, id)
		it.Must.Nil(err)
		it.Must.True(found, errMessage)
	})
	return ptr
}

func IsAbsent[T any, ID any](tb testing.TB, subject frameless.Finder[T, ID], ctx context.Context, id ID) {
	tb.Helper()
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be absent", *new(T), id)
	Eventually.Assert(tb, func(it assert.It) {
		found, err := subject.FindByID(ctx, new(T), id)
		it.Must.Nil(err)
		it.Must.False(found, errMessage)
	})
}

func HasEntity[T any, ID any](tb testing.TB, subject frameless.Finder[T, ID], ctx context.Context, ptr *T) {
	tb.Helper()
	id := HasID[T, ID](tb, ptr)
	Eventually.Assert(tb, func(it assert.It) {
		// IsFindable yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		it.Must.Equal(ptr, IsFindable(it, subject, ctx, id))
	})
}

func Create[T any, ID any](tb testing.TB, subject resource[T, ID], ctx context.Context, ptr *T) {
	tb.Helper()

	assert.Must(tb).Nil(subject.Create(ctx, ptr))
	id := HasID[T, ID](tb, ptr)
	tb.Cleanup(func() {
		found, err := subject.FindByID(ctx, new(T), id)
		if err != nil || !found {
			return
		}
		_ = subject.DeleteByID(ctx, id)
	})
	IsFindable[T, ID](tb, subject, ctx, id)
	tb.Logf("given entity is created: %#v", ptr)
}

func Update[T any, ID any](tb testing.TB, subject interface {
	frameless.Finder[T, ID]
	frameless.Updater[T]
	frameless.Deleter[ID]
}, ctx context.Context, ptr *T) {
	tb.Helper()
	id, _ := extid.Lookup[ID](ptr)
	// IsFindable ensures that by the time Update is executed,
	// the entity is present in the resource.
	IsFindable[T, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.Update(ctx, ptr))
	Eventually.Assert(tb, func(it assert.It) {
		entity := IsFindable[T, ID](it, subject, ctx, id)
		it.Must.Equal(ptr, entity)
	})
	tb.Logf(`entity is updated: %#v`, ptr)
}

func Delete[T, ID any](tb testing.TB, subject resource[T, ID], ctx context.Context, ptr *T) {
	tb.Helper()
	id := HasID[T, ID](tb, ptr)
	IsFindable[T, ID](tb, subject, ctx, id)
	assert.Must(tb).Nil(subject.DeleteByID(ctx, id))
	IsAbsent[T, ID](tb, subject, ctx, id)
	tb.Logf("entity is deleted: %#v", ptr)
}

func DeleteAll[T any, ID any](tb testing.TB, subject resource[T, ID], ctx context.Context) {
	tb.Helper()
	assert.Must(tb).Nil(subject.DeleteAll(ctx))
	Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	Eventually.Assert(tb, func(it assert.It) {
		count, err := iterators.Count(subject.FindAll(ctx))
		it.Must.Nil(err)
		it.Should.True(count == 0, `no entity was expected to be found`)
	})
}

func CountIs[T any](tb testing.TB, iter frameless.Iterator[T], expected int) {
	tb.Helper()
	count, err := iterators.Count(iter)
	assert.Must(tb).Nil(err)
	assert.Must(tb).Equal(expected, count)
}
