package guardcontracts_test

import (
	"context"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/guard/guardcontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestLocker_memory(t *testing.T) {
	guardcontracts.Locker(func(tb testing.TB) guardcontracts.LockerSubject {
		_, ok := tb.(*testcase.T)
		assert.True(tb, ok)
		return guardcontracts.LockerSubject{
			Locker:      memory.NewLocker(),
			MakeContext: context.Background,
		}
	}).Test(t)
}

func TestLockerFactory_memory(t *testing.T) {
	guardcontracts.LockerFactory[string](func(tb testing.TB) guardcontracts.LockerFactorySubject[string] {
		return guardcontracts.LockerFactorySubject[string]{
			LockerFactory:     memory.NewLockerFactory[string](),
			MakeContext: context.Background,
			MakeKey:     tb.(*testcase.T).Random.String,
		}
	}).Test(t)
}

func TestLockerFactory_memory_factoryFunc(t *testing.T) {
	guardcontracts.LockerFactory[string](func(tb testing.TB) guardcontracts.LockerFactorySubject[string] {
		return guardcontracts.LockerFactorySubject[string]{
			LockerFactory:     memory.NewLockerFactory[string](),
			MakeContext: context.Background,
			MakeKey: func() string {
				return tb.(*testcase.T).Random.String()
			},
		}
	}).Test(t)
}
