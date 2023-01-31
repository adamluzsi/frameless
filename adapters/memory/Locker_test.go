package memory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/lockscontracts"
)

func ExampleLocker() {
	l := memory.NewLocker()

	ctx, err := l.Lock(context.Background())
	if err != nil {
		panic(err)
	}

	if err := l.Unlock(ctx); err != nil {
		panic(err)
	}
}

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
