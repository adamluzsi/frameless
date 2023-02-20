package logger_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"testing"
	"time"
)

func TestContextWithDetails(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze())

	t.Run("on nil details, original context is returned", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "key", "value")
		assert.Equal(t, ctx, logger.ContextWithDetails(ctx, nil))
	})

	t.Run("logging details can be added to the context", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx := context.Background()
		ctx = logger.ContextWithDetails(ctx, logger.Details{"foo": "bar"})
		ctx = logger.ContextWithDetails(ctx, logger.Details{"bar": 42})
		ctx = logger.ContextWithDetails(ctx, logger.Details{"numbers": []int{1, 2, 3}, "hello": "world"})

		logger.Info(ctx, "msg")
		assert.Contain(t, buf.String(), `"level":"info"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"hello":"world"`)
		assert.Contain(t, buf.String(), `"numbers":[1,2,3]`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
	})

	t.Run("contexts are isolated and not leaking out between each other", func(t *testing.T) {
		buf := logger.Stub(t)
		ctx0 := context.Background()
		ctx1 := logger.ContextWithDetails(ctx0, logger.Details{"foo": "bar"})
		ctx2 := logger.ContextWithDetails(ctx1, logger.Details{"bar": 42})

		logger.Info(ctx1, "42")
		assert.Contain(t, buf.String(), `"message":"42"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.NotContain(t, buf.String(), `"bar":42`)

		logger.Info(ctx2, "24")
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"bar":42`)
	})
}
