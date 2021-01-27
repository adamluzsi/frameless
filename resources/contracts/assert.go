package contracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var Waiter = testcase.Waiter{
	WaitDuration: time.Millisecond,
	WaitTimeout:  5 * time.Second,
}

var AsyncTester = testcase.Retry{Strategy: Waiter}

type NewEntityFunc func() interface{}

func HasID(tb testing.TB, ent interface{}) (id interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var ok bool
		id, ok = resources.LookupID(ent)
		require.True(tb, ok)
		require.NotEmpty(tb, id)
	})
	return
}

func IsFindable(tb testing.TB, subject resources.Finder, ctx context.Context, newEntity NewEntityFunc, id interface{}) interface{} {
	var ptr interface{}
	AsyncTester.Assert(tb, func(tb testing.TB) {
		ptr = newEntity()
		found, err := subject.FindByID(ctx, ptr, id)
		require.Nil(tb, err)
		require.True(tb, found)
	})
	return ptr
}

func IsAbsent(tb testing.TB, subject resources.Finder, ctx context.Context, newEntity NewEntityFunc, id interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		found, err := subject.FindByID(ctx, newEntity(), id)
		require.Nil(tb, err)
		require.False(tb, found)
	})
}

func HasEntity(tb testing.TB, subject resources.Finder, ctx context.Context, ent interface{}) {
	T := toT(ent)
	id := HasID(tb, ent)
	newFunc := newEntityFunc(T)
	AsyncTester.Assert(tb, func(tb testing.TB) {
		require.Equal(tb, ent, IsFindable(tb, subject, ctx, newFunc, id))
	})
}

func CreateEntity(tb testing.TB, subject minimumRequirements, ctx context.Context, ptr interface{}) {
	T := toT(ptr)
	require.Nil(tb, subject.Create(ctx, ptr))
	id := HasID(tb, ptr)
	tb.Cleanup(func() { _ = subject.DeleteByID(ctx, T, id) })
	IsFindable(tb, subject, ctx, newEntityFunc(T), id)
}

func UpdateEntity(tb testing.TB, subject interface {
	resources.Finder
	resources.Updater
	resources.Deleter
}, ctx context.Context, ptr interface{}) {
	T := toT(ptr)
	id, _ := resources.LookupID(ptr)
	require.Nil(tb, subject.Update(ctx, ptr))
	AsyncTester.Assert(tb, func(tb testing.TB) {
		entity := IsFindable(tb, subject, ctx, newEntityFunc(T), id)
		require.Equal(tb, ptr, entity)
	})
}

func DeleteEntity(tb testing.TB, subject minimumRequirements, ctx context.Context, ent interface{}) {
	T := toT(ent)
	id := HasID(tb, ent)
	IsFindable(tb, subject, ctx, newEntityFunc(T), id)
	require.Nil(tb, subject.DeleteByID(ctx, T, id))
	IsAbsent(tb, subject, ctx, newEntityFunc(T), id)
}

func DeleteAllEntity(tb testing.TB, subject minimumRequirements, ctx context.Context, T resources.T) {
	getCount := func(tb testing.TB) int {
		count, err := iterators.Count(subject.FindAll(ctx, T))
		require.Nil(tb, err)
		return count
	}

	if getCount(tb) == 0 {
		return
	}

	require.Nil(tb, subject.DeleteAll(ctx, T))

	AsyncTester.Assert(tb, func(tb testing.TB) {
		require.True(tb, getCount(tb) == 0, fmt.Sprintf(`no %T was expected to be found in %T`, T, subject))
	})
}
