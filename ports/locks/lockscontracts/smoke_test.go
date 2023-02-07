package lockscontracts_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/frameless/ports/locks/lockscontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestLocker_memory(t *testing.T) {
	lockscontracts.Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			_, ok := tb.(*testcase.T)
			assert.True(tb, ok)
			return memory.NewLocker()
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}

func TestLockerFactory_memory(t *testing.T) {
	lockscontracts.Factory[string]{
		MakeSubject: func(tb testing.TB) locks.Factory[string] {
			return memory.NewLockerFactory[string]()
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeKey: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	}.Test(t)
}

func TestLockerFactory_memory_factoryFunc(t *testing.T) {
	lockscontracts.Factory[string]{
		MakeSubject: func(tb testing.TB) locks.Factory[string] {
			return locks.FactoryFunc[string](memory.NewLockerFactory[string]().LockerFor)
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeKey: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	}.Test(t)
}
