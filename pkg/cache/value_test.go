package cache_test

import (
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/random"
	"testing"
	"time"
)

var rnd = random.New(random.CryptoSeed{})

func TestMemTTL(t *testing.T) {
	var (
		now  = time.Now()
		val1 = rnd.String()
		val2 = rnd.String()
		val3 = rnd.String()
	)
	NewSequence := func() func() string {
		var i int
		return func() (r string) {
			i++
			switch i {
			case 1:
				return val1
			case 2:
				return val2
			case 3:
				return val3
			default:
				return random.Unique(rnd.String,
					val1, val2, val3)
			}
		}
	}
	{
		seq := NewSequence()
		assert.Equal(t, val1, seq())
		assert.Equal(t, val2, seq())
		assert.Equal(t, val3, seq())
	}
	t.Run(".Do + Invalidate", func(t *testing.T) {
		timecop.Travel(t, now, timecop.Freeze())
		var (
			sequence  = NewSequence()
			cachedVal = cache.Value[string]{}
			act       = func() string {
				v, err := cachedVal.Do(func() (string, error) {
					return sequence(), nil
				})
				assert.NoError(t, err)
				return v
			}
		)
		assert.Equal(t, act(), val1, "initial call will trigger mem caching")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		cachedVal.Invalidate()
		assert.Equal(t, act(), val2, "after invalidation, the new value is taken")
	})
	t.Run(".Do", func(t *testing.T) {
		timecop.Travel(t, now, timecop.Freeze())
		var (
			seq   = NewSequence()
			store = cache.Value[string]{}
			act   = func() string {
				v, err := store.Do(func() (string, error) {
					return seq(), nil
				})
				assert.NoError(t, err)
				return v
			}
		)
		assert.Equal(t, act(), val1, "initial call will trigger mem caching")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		timecop.Travel(t, time.Hour*24, timecop.Freeze())
		assert.Equal(t, act(), val1, "regardless of the time, the value remains the same")
	})
	t.Run(".Do on error, value is not cached", func(t *testing.T) {
		timecop.Travel(t, now, timecop.Freeze())
		var (
			seq       = NewSequence()
			cachedVal = cache.Value[string]{}
			expErr    = rnd.Error()
			doError   = true
			act       = func() (string, error) {
				return cachedVal.Do(func() (string, error) {
					if doError {
						return "", expErr
					}
					return seq(), nil
				})
			}
		)

		_, gotErr := act()
		assert.ErrorIs(t, expErr, gotErr)
		_, gotErr = act()
		assert.ErrorIs(t, expErr, gotErr)

		doError = false

		gotVal, gotErr := act()
		assert.NoError(t, gotErr)
		assert.Equal(t, gotVal, val1)

		doError = true // enable error return again

		gotVal, gotErr = act()
		assert.NoError(t, gotErr)
		assert.Equal(t, gotVal, val1)
	})
	t.Run(".Do with TTL", func(t *testing.T) {
		timecop.Travel(t, now, timecop.Freeze())
		var (
			seq       = NewSequence()
			ttl       = time.Hour
			cachedVal = cache.Value[string]{TTL: ttl}
			act       = func() string {
				v, err := cachedVal.Do(func() (string, error) {
					return seq(), nil
				})
				assert.NoError(t, err)
				return v
			}
		)
		assert.Equal(t, act(), val1, "initial call will trigger mem caching")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		assert.Equal(t, act(), val1, "following requests will use the cached value")
		timecop.Travel(t, ttl, timecop.Freeze())
		assert.Equal(t, act(), val2, "cache should be invalidated due to TTL")
		timecop.Travel(t, ttl+time.Second, timecop.Freeze())
		assert.Equal(t, act(), val3, "cache should be invalidated again because the time due to TTL")
		timecop.Travel(t, ttl-time.Second, timecop.Freeze())
		assert.Equal(t, act(), val3, "we are at the end of the TTL, so the cached value is still returned")
	})
}
