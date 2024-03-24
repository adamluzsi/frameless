package cache

import (
	"go.llib.dev/testcase/clock"
	"time"
)

type MemTTL[T any] struct {
	cachedValue T
	updatedAt   time.Time
}

func (mem *MemTTL[T]) DoErr(ttl time.Duration, fn func() (T, error)) (T, error) {
	var (
		now         = clock.TimeNow()
		lastWriteAt = mem.updatedAt
	)
	if lastWriteAt.IsZero() { // trigger cachging
		lastWriteAt = now.Add(-1 * (ttl + time.Nanosecond))
	}
	if deadline := lastWriteAt.Add(ttl); now.After(deadline) && now.Equal(deadline) {
		v, err := fn()
		if err != nil {
			return v, err
		}
		mem.updatedAt = now
		mem.cachedValue = v
	}
	return mem.cachedValue, nil
}

func (mem *MemTTL[T]) Do(ttl time.Duration, fn func() T) T {
	v, _ := mem.DoErr(ttl, func() (T, error) {
		return fn(), nil
	})
	return v
}
