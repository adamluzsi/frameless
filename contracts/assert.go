package contracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var Waiter = testcase.Waiter{
	WaitDuration: time.Millisecond,
	WaitTimeout:  5 * time.Second,
}

var AsyncTester = testcase.Retry{Strategy: Waiter}

func HasID(tb testing.TB, ent interface{}) (id interface{}) {
	tb.Helper()
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var ok bool
		id, ok = frameless.LookupID(ent)
		require.True(tb, ok)
		require.NotEmpty(tb, id)
	})
	return
}

func IsFindable(tb testing.TB, T T, subject frameless.Finder, ctx context.Context, id interface{}) interface{} {
	tb.Helper()
	var ptr interface{}
	newFn := newEntityFunc(T)
	AsyncTester.Assert(tb, func(tb testing.TB) {
		ptr = newFn()
		found, err := subject.FindByID(ctx, ptr, id)
		require.Nil(tb, err)
		require.True(tb, found)
	})
	return ptr
}

func IsAbsent(tb testing.TB, T T, subject frameless.Finder, ctx context.Context, id interface{}) {
	tb.Helper()
	n := newEntityFunc(T)
	AsyncTester.Assert(tb, func(tb testing.TB) {
		found, err := subject.FindByID(ctx, n(), id)
		require.Nil(tb, err)
		require.False(tb, found)
	})
}

func HasEntity(tb testing.TB, subject frameless.Finder, ctx context.Context, ent interface{}) {
	tb.Helper()
	T := toT(ent)
	id := HasID(tb, ent)
	AsyncTester.Assert(tb, func(tb testing.TB) {
		// IsFindable yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		require.Equal(tb, ent, IsFindable(tb, T, subject, ctx, id))
	})
}

func CreateEntity(tb testing.TB, subject CRD, ctx context.Context, ptr interface{}) {
	tb.Helper()
	T := toT(ptr)
	require.Nil(tb, subject.Create(ctx, ptr))
	id := HasID(tb, ptr)
	tb.Cleanup(func() { _ = subject.DeleteByID(ctx, T, id) })
	IsFindable(tb, T, subject, ctx, id)
}

func UpdateEntity(tb testing.TB, subject interface {
	frameless.Finder
	frameless.Updater
	frameless.Deleter
}, ctx context.Context, ptr interface{}) {
	tb.Helper()
	T := toT(ptr)
	id, _ := frameless.LookupID(ptr)
	// IsFindable ensures that by the time Update is executed,
	// the entity is present in the resource.
	IsFindable(tb, T, subject, ctx, id)
	require.Nil(tb, subject.Update(ctx, ptr))
	AsyncTester.Assert(tb, func(tb testing.TB) {
		entity := IsFindable(tb, T, subject, ctx, id)
		require.Equal(tb, ptr, entity)
	})
}

func DeleteEntity(tb testing.TB, subject CRD, ctx context.Context, ent interface{}) {
	tb.Helper()
	T := toT(ent)
	id := HasID(tb, ent)
	IsFindable(tb, T, subject, ctx, id)
	require.Nil(tb, subject.DeleteByID(ctx, T, id))
	IsAbsent(tb, T, subject, ctx, id)
}

func DeleteAllEntity(tb testing.TB, subject CRD, ctx context.Context, T frameless.T) {
	tb.Helper()
	require.Nil(tb, subject.DeleteAll(ctx, T))
	Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	AsyncTester.Assert(tb, func(tb testing.TB) {
		count, err := iterators.Count(subject.FindAll(ctx, T))
		require.Nil(tb, err)
		require.True(tb, count == 0, fmt.Sprintf(`no %T was expected to be found in %T`, T, subject))
	})
}
