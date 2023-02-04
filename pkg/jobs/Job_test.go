package jobs

import (
	"context"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestToJob(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on Job", func(t *testing.T) {
		assert.NotNil(t, toJob(Job(func(ctx context.Context) error { return nil })))
		expErr := rnd.Error()
		assert.Equal(t, expErr, toJob(Job(func(ctx context.Context) error { return expErr }))(context.Background()))
		assert.NoError(t, toJob(Job(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		}))(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on Job func", func(t *testing.T) {
		assert.NotNil(t, toJob(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, toJob(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, toJob(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		})(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, toJob(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, toJob(func() error { return expErr })(context.Background()))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, toJob(func() {}))
		assert.NoError(t, toJob(func() {})(context.Background()))
	})
}
