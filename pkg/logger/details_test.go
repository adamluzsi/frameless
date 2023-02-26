package logger_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/pkg/logger/logdto"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"testing"
	"time"
)

func TestDetails_Merge(t *testing.T) {
	t.Run("on populated details", func(t *testing.T) {
		d := logger.Details{"foo": "bar", "bar": "foo"}
		gotD := d.Merge(logger.Details{"bar": "baz", "answer": 42})
		assert.Equal(t, d, gotD)
		assert.Equal(t, logger.Details{
			"foo":    "bar",
			"bar":    "baz",
			"answer": 42,
		}, d)
	})
	t.Run("on nil details", func(t *testing.T) {
		d := logger.Details{"foo": "bar", "bar": "foo"}
		d.Merge(nil)
		assert.Equal(t, logger.Details{"foo": "bar", "bar": "foo"}, d)
	})
}

func ExampleDetails_Err() {
	ctx := context.Background()
	err := errors.New("boom")

	logger.Error(ctx, "task failed successfully", logger.Details{}.Err(err))
}

func TestDetails_Err(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("plain error", func(t *testing.T) {
		expErr := rnd.Error()
		d := logger.Details{}.Err(expErr)
		assert.Equal(t, logger.Details{"error": logdto.Error{Message: expErr.Error()}}, d)
	})
	t.Run("when err is a user error", func(t *testing.T) {
		const message = "The answer"
		const code = "42"
		var expErr error
		expErr = errorutil.UserError{ID: code, Message: message}
		expErr = fmt.Errorf("err: %w", expErr)
		d := logger.Details{}.Err(expErr)
		assert.Equal(t, logger.Details{"error": logdto.Error{
			Message: expErr.Error(),
			Code:    code,
			Detail:  message,
		}}, d)
	})
	t.Run("when err has details", func(t *testing.T) {
		const detail = "Hello, world!"
		var expErr error
		expErr = rnd.Error()
		expErr = errorutil.With(expErr).Detail(detail)
		d := logger.Details{}.Err(expErr)
		assert.Equal(t, logger.Details{"error": logdto.Error{
			Message: expErr.Error(),
			Detail:  detail,
		}}, d)
	})
}

func ExampleContextWithDetails() {
	ctx := context.Background()

	ctx = logger.ContextWithDetails(ctx, logger.Details{
		"foo": "bar",
		"baz": "qux",
	})

	logger.Info(ctx, "message") // will have details from the context
}

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
