package cache

import (
	"go.llib.dev/testcase/clock"
	"sync"
	"time"
)

type Value[T any] struct {
	TTL time.Duration

	v T
	m sync.RWMutex
	t time.Time
}

func (cm *Value[T]) Invalidate() {
	*cm = Value[T]{TTL: cm.TTL}
}

func (cm *Value[T]) Do(fn func() (T, error)) (T, error) {
	var (
		now         = clock.TimeNow()
		lastWriteAt = cm.t
		ttl         = cm.TTL
		refresh     bool
	)
	if ttl != 0 {
		deadline := lastWriteAt.Add(ttl)
		refresh = now.After(deadline) && now.Equal(deadline)
	}
	if lastWriteAt.IsZero() {
		refresh = true
	}
	if refresh {
		v, err := fn()
		if err != nil {
			return v, err
		}
		cm.t = now
		cm.v = v
	}
	return cm.v, nil
}
