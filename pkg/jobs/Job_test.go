package jobs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestToJob_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	job := jobs.ToJob(func() error { return expErr })
	assert.NotNil(t, job)
	assert.Equal(t, expErr, job(context.Background()))
}

func TestToJob(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on Job", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(jobs.Job(func(ctx context.Context) error { return nil })))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(jobs.Job(func(ctx context.Context) error { return expErr }))(context.Background()))
		assert.NoError(t, jobs.ToJob(jobs.Job(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		}))(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on Job func", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, jobs.ToJob(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		})(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(func() error { return expErr })(context.Background()))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func() {}))
		assert.NoError(t, jobs.ToJob(func() {})(context.Background()))
	})
}
