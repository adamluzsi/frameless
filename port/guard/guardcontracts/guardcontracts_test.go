package guardcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard/guardcontracts"
)

func Test_memory(t *testing.T) {
	t.Run("Lock", func(t *testing.T) {
		l := memory.NewLocker()
		guardcontracts.Locker(l).Test(t)
		guardcontracts.NonBlockingLocker(l).Test(t)
	})
	t.Run("LockerFactory", func(t *testing.T) {
		f := memory.NewLockerFactory[string]()
		guardcontracts.LockerFactory[string](f).Test(t)
		guardcontracts.NonBlockingLockerFactory[string](f).Test(t)
	})
}
