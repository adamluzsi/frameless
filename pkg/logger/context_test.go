package logger_test

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"testing"
	"time"
)

func ExampleContextWith() {
	ctx := context.Background()

	ctx = logger.ContextWith(ctx, logger.Fields{
		"foo": "bar",
		"baz": "qux",
	})

	logger.Info(ctx, "message") // will have details from the context
}

func TestContextWith(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())

	t.Run("on nil details, original context is returned", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "key", "value")
		assert.Equal(t, ctx, logger.ContextWith(ctx))
	})

	t.Run("logging details can be added to the context", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx := context.Background()
		ctx = logger.ContextWith(ctx, logger.Fields{"foo": "bar"})
		ctx = logger.ContextWith(ctx, logger.Fields{"bar": 42})
		ctx = logger.ContextWith(ctx, logger.Fields{"numbers": []int{1, 2, 3}, "hello": "world"})

		logger.Info(ctx, "msg")
		assert.Contain(t, buf.String(), `"level":"info"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"hello":"world"`)
		assert.Contain(t, buf.String(), `"numbers":[1,2,3]`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
	})

	t.Run("contextkit are isolated and not leaking out between each other", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx0 := context.Background()
		ctx1 := logger.ContextWith(ctx0, logger.Fields{"foo": "bar"})
		ctx2 := logger.ContextWith(ctx1, logger.Fields{"bar": 42})

		logger.Info(ctx1, "42")
		assert.Contain(t, buf.String(), `"message":"42"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.NotContain(t, buf.String(), `"bar":42`)

		logger.Info(ctx2, "24")
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"bar":42`)
	})
}
