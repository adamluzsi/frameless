package iterators

import (
	"context"
	"sync"
	"time"
)

func Batch[T any](i Iterator[T], c BatchConfig) *BatchIter[T] {
	return &BatchIter[T]{Iterator: i, Config: c}
}

type BatchConfig struct {
	// Size is the max amount of element that a batch will contains.
	// Default batch Size is 100.
	Size int
	// Timeout is batching wait timout duration that the batching process is willing to wait for, before starting to build a new batch.
	// Default batch Timeout is 100 Millisecond.
	Timeout time.Duration
}

func (c BatchConfig) getTimeout() time.Duration {
	const defaultTimeout = 100 * time.Millisecond
	if c.Timeout <= 0 {
		return defaultTimeout
	}
	return c.Timeout
}

func (c BatchConfig) getSize() int {
	const defaultSize = 100
	if c.Size <= 0 {
		return defaultSize
	}
	return c.Size
}

type BatchIter[T any] struct {
	Iterator Iterator[T]
	Config   BatchConfig

	init   sync.Once
	stream chan T
	cancel func()

	batch []T
}

func (i *BatchIter[T]) Init() {
	i.init.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		i.stream = make(chan T)
		i.cancel = cancel
		go i.fetch(ctx)
	})
}

func (i *BatchIter[T]) fetch(ctx context.Context) {
wrk:
	for i.Iterator.Next() {
		select {
		case <-ctx.Done():
			break wrk
		case i.stream <- i.Iterator.Value():
		}
	}
}

func (i *BatchIter[T]) Close() error {
	i.init.Do(func() {}) // prevent async interactions
	if i.cancel != nil {
		i.cancel()
	}
	return i.Iterator.Close()
}

// Err return the cause if for some reason by default the More return false all the time
func (i *BatchIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *BatchIter[T]) Next() bool {
	i.Init()

	size := i.Config.getSize()
	i.batch = make([]T, 0, size)
	timer := time.NewTimer(i.Config.getTimeout())
	defer stopTimer(timer)

batching:
	for len(i.batch) < size {
		resetTimer(timer, i.Config.getTimeout())

		select {
		case v, open := <-i.stream:
			if !open {
				break batching
			}
			i.batch = append(i.batch, v)

		case <-timer.C:
			break batching

		}
	}

	return 0 < len(i.batch)
}

// Value returns the current value in the iterator.
// The action should be repeatable without side effect.
func (i *BatchIter[T]) Value() []T {
	return i.batch
}

func stopTimer(timer *time.Timer) {
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
}

func resetTimer(timer *time.Timer, timeout time.Duration) {
	stopTimer(timer)
	timer.Reset(timeout)
}
