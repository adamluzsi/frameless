package guardcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard/guardcontracts"
)

func Test_memory(t *testing.T) {
	l := memory.NewLocker()
	f := memory.NewLockerFactory[string]()
	t.Run("Locker", guardcontracts.Locker(l).Test)
	t.Run("NonBlockingLocker", guardcontracts.NonBlockingLocker(l).Test)
	t.Run("LockerFactory", guardcontracts.LockerFactory[string](f).Test)
	t.Run("NonBlockingLockerFactory", guardcontracts.NonBlockingLockerFactory[string](f).Test)
}
