package httpkit_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/testcase/assert"
)

func TestWithPathParam(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := httpkit.WithPathParam(ctx, "foo", "A")
		ctx2 := httpkit.WithPathParam(ctx1, "bar", "B")
		ctx3 := httpkit.WithPathParam(ctx2, "foo", "C")

		assert.Equal(t, httpkit.PathParams(ctx), map[string]string{})
		assert.Equal(t, httpkit.PathParams(ctx1), map[string]string{
			"foo": "A",
		})
		assert.Equal(t, httpkit.PathParams(ctx2), map[string]string{
			"foo": "A",
			"bar": "B",
		})
		assert.Equal(t, httpkit.PathParams(ctx3), map[string]string{
			"foo": "C",
			"bar": "B",
		})
	})
	t.Run("variable can be set in the context", func(t *testing.T) {
		ctx := context.Background()
		ctx = httpkit.WithPathParam(ctx, "foo", "A")
		assert.Equal(t, httpkit.PathParams(ctx), map[string]string{"foo": "A"})
	})
	t.Run("variable can be overwritten with WithPathParam", func(t *testing.T) {
		ctx := context.Background()
		ctx = httpkit.WithPathParam(ctx, "foo", "A")
		ctx = httpkit.WithPathParam(ctx, "foo", "C")
		assert.Equal(t, httpkit.PathParams(ctx), map[string]string{"foo": "C"})
	})
	t.Run("variable overwriting is not mutating the original context", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := httpkit.WithPathParam(ctx, "foo", "A")
		ctx2 := httpkit.WithPathParam(ctx1, "foo", "C")
		assert.Equal(t, httpkit.PathParams(ctx1), map[string]string{"foo": "A"})
		assert.Equal(t, httpkit.PathParams(ctx2), map[string]string{"foo": "C"})
	})
}
