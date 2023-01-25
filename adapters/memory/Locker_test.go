package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/contracts"
	"testing"
)

func TestLocker(t *testing.T) {
	lockscontracts.Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			return memory.NewLocker()
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}
