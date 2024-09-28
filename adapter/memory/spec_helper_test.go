package memory_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

var (
	waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      3 * time.Second,
	}
	eventually = assert.Retry{
		Strategy: waiter,
	}
)

// Entity is an example entity that can be used for testing
type TestEntity struct {
	ID   string `ext:"id"`
	Data string
	List []string
}

func makeTestEntityFunc(tb testing.TB) func() TestEntity {
	return func() TestEntity { return makeTestEntity(tb) }
}

func makeTestEntity(tb testing.TB) TestEntity {
	t := tb.(*testcase.T)
	var list []string
	n := t.Random.IntBetween(1, 3)
	for i := 0; i < n; i++ {
		list = append(list, t.Random.String())
	}
	return TestEntity{
		Data: t.Random.String(),
		List: list,
	}
}

func makeContext(testing.TB) context.Context {
	return context.Background()
}
