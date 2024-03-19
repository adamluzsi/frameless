package memory_test

import (
	"go.llib.dev/frameless/adapters/memory"
	. "go.llib.dev/frameless/spechelper/testent"
	"testing"
)

func TestQuery(t *testing.T) {
	mem := memory.NewMemory()
	fooRepo := memory.NewRepository[Foo, FooID](mem)

}
