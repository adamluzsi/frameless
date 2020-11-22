package specs

import (
	"context"
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var AsyncTester = testcase.AsyncTester{
	WaitDuration: time.Microsecond,
	WaitTimeout:  time.Minute,
}

var Waiter = AsyncTester

type NewEntityFunc func() interface{}

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
