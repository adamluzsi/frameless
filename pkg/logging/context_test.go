package logging_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
)

func ExampleContextWith() {
	ctx := context.Background()

	ctx = logging.ContextWith(ctx, logging.Fields{
		"foo": "bar",
		"baz": "qux",
	})

	l := &logging.Logger{}
	l.Info(ctx, "message") // will have details from the context
}

func TestContextWith(t *testing.T) {
	now := time.Now()
	timecop.Travel(t, now, timecop.Freeze)

	t.Run("on nil details, original context is returned", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "key", "value")
		assert.Equal(t, ctx, logging.ContextWith(ctx))
	})

	t.Run("logging details can be added to the context", func(t *testing.T) {
		l, buf := logging.Stub(t)
		ctx := context.Background()
		ctx = logging.ContextWith(ctx, logging.Fields{"foo": "bar"})
		ctx = logging.ContextWith(ctx, logging.Fields{"bar": 42})
		ctx = logging.ContextWith(ctx, logging.Fields{"numbers": []int{1, 2, 3}, "hello": "world"})

		l.Info(ctx, "msg")
		assert.Contain(t, buf.String(), `"level":"info"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"hello":"world"`)
		assert.Contain(t, buf.String(), `"numbers":[1,2,3]`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"timestamp":"%s"`, now.Format(time.RFC3339)))
	})

	t.Run("contextkit are isolated and not leaking out between each other", func(t *testing.T) {
		l, buf := logging.Stub(t)
		ctx0 := context.Background()
		ctx1 := logging.ContextWith(ctx0, logging.Fields{"foo": "bar"})
		ctx2 := logging.ContextWith(ctx1, logging.Fields{"bar": 42})

		l.Info(ctx1, "42")
		assert.Contain(t, buf.String(), `"message":"42"`)
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.NotContain(t, buf.String(), `"bar":42`)

		l.Info(ctx2, "24")
		assert.Contain(t, buf.String(), `"foo":"bar"`)
		assert.Contain(t, buf.String(), `"bar":42`)
	})
}
