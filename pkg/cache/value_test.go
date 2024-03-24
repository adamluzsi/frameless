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
		return func() string {
			switch i {
			case 1:
				return val1
			case 2:
				return val2
			case 3:
				return val3
			default:
				return rnd.String()
			}
		}
	}
	t.Run(".Do", func(t *testing.T) {
		timecop.Travel(t, now, timecop.Freeze())
		var (
			store cache.MemTTL[string]
			seq   = NewSequence()
			ttl   = time.Hour
			act   = func() string {
				return store.Do(ttl, seq)
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
