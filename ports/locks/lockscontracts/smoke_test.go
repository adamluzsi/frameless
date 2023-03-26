package lockscontracts_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks/lockscontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestLocker_memory(t *testing.T) {
	lockscontracts.Locker(func(tb testing.TB) lockscontracts.LockerSubject {
		_, ok := tb.(*testcase.T)
		assert.True(tb, ok)
		return lockscontracts.LockerSubject{
			Locker:      memory.NewLocker(),
			MakeContext: context.Background,
		}
	}).Test(t)
}

func TestLockerFactory_memory(t *testing.T) {
	lockscontracts.Factory[string](func(tb testing.TB) lockscontracts.FactorySubject[string] {
		return lockscontracts.FactorySubject[string]{
			Factory:     memory.NewLockerFactory[string](),
			MakeContext: context.Background,
			MakeKey:     tb.(*testcase.T).Random.String,
		}
	}).Test(t)
}

func TestLockerFactory_memory_factoryFunc(t *testing.T) {
	lockscontracts.Factory[string](func(tb testing.TB) lockscontracts.FactorySubject[string] {
		return lockscontracts.FactorySubject[string]{
			Factory:     memory.NewLockerFactory[string](),
			MakeContext: context.Background,
			MakeKey: func() string {
				return tb.(*testcase.T).Random.String()
			},
		}
	}).Test(t)
}
