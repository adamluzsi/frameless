package guardcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard/guardcontracts"
)

func TestLocker_memory(t *testing.T) {
	guardcontracts.Locker(memory.NewLocker()).Test(t)
}

func TestLockerFactory_memory(t *testing.T) {
	guardcontracts.LockerFactory[string](memory.NewLockerFactory[string]()).Test(t)
}
