package lockscontracts_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/contracts"
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
