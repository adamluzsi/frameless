package guardcontract_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard/guardcontract"
)

func Test_memory(t *testing.T) {
	l := memory.NewLocker()
	f := memory.NewLockerFactory[string]()
	t.Run("Locker", guardcontract.Locker(l).Test)
	t.Run("NonBlockingLocker", guardcontract.NonBlockingLocker(l).Test)
	t.Run("LockerFactory", guardcontract.LockerFactory[string](f).Test)
	t.Run("NonBlockingLockerFactory", guardcontract.NonBlockingLockerFactory[string](f).Test)
}
