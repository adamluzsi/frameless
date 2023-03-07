package iterators

import (
	"context"
	"sync"
	"time"
)

func Batch[T any](iter Iterator[T], size int) Iterator[[]T] {
	return &batchIter[T]{
		Iterator: iter,
		Size:     size,
	}
}

type batchIter[T any] struct {
	Iterator Iterator[T]
	// Size is the max amount of element that a batch will contains.
	// Default batch Size is 100.
	Size int

	values []T
	done   bool
	closed bool
}

func (i *batchIter[T]) Close() error {
	i.closed = true
	return i.Iterator.Close()
}

func (i *batchIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *batchIter[T]) Next() bool {
	if i.closed {
		return false
	}
	if i.done {
		return false
	}
	batchSize := getBatchSize(i.Size)
	i.values = make([]T, 0, batchSize)
	for {
		hasNext := i.Iterator.Next()
		if !hasNext {
			i.done = true
			break
		}
		i.values = append(i.values, i.Iterator.Value())
		if batchSize <= len(i.values) {
			break
		}
	}
	return 0 < len(i.values)
}

func (i *batchIter[T]) Value() []T {
	return i.values
}

func BatchWithTimeout[T any](i Iterator[T], size int, timeout time.Duration) Iterator[[]T] {
	return &batchWithTimeoutIter[T]{
		Iterator: i,
		Size:     size,
		Timeout:  timeout,
	}
}

type batchWithTimeoutIter[T any] struct {
	Iterator Iterator[T]
	// Size is the max amount of element that a batch will contains.
	// Default batch Size is 100.
	Size int
	// Timeout is batching wait timout duration that the batching process is willing to wait for, before starting to build a new batch.
	// Default batch Timeout is 100 Millisecond.
	Timeout time.Duration

	init   sync.Once
	stream chan T
	cancel func()

	batch []T
}

func (i *batchWithTimeoutIter[T]) Init() {
	i.init.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		i.stream = make(chan T)
		i.cancel = cancel
		go i.fetch(ctx)
	})
}

func (i *batchWithTimeoutIter[T]) fetch(ctx context.Context) {
wrk:
	for i.Iterator.Next() {
		select {
		case <-ctx.Done():
			break wrk
		case i.stream <- i.Iterator.Value():
		}
	}
}

func (i *batchWithTimeoutIter[T]) Close() error {
	i.init.Do(func() {}) // prevent async interactions
	if i.cancel != nil {
		i.cancel()
	}
	return i.Iterator.Close()
}

// Err return the cause if for some reason by default the More return false all the time
func (i *batchWithTimeoutIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *batchWithTimeoutIter[T]) Next() bool {
	i.Init()

	size := getBatchSize(i.Size)
	i.batch = make([]T, 0, size)

	timer := time.NewTimer(i.lookupTimeout())
	defer i.stopTimer(timer)

batching:
	for len(i.batch) < size {
		i.resetTimer(timer, i.lookupTimeout())

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
func (i *batchWithTimeoutIter[T]) Value() []T {
	return i.batch
}

func (i *batchWithTimeoutIter[T]) lookupTimeout() time.Duration {
	const defaultTimeout = 100 * time.Millisecond
	if i.Timeout <= 0 {
		return defaultTimeout
	}
	return i.Timeout
}

func (i *batchWithTimeoutIter[T]) stopTimer(timer *time.Timer) {
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
}

func (i *batchWithTimeoutIter[T]) resetTimer(timer *time.Timer, timeout time.Duration) {
	i.stopTimer(timer)
	timer.Reset(timeout)
}

func getBatchSize(size int) int {
	const defaultBatchSize = 64
	if size <= 0 {
		return defaultBatchSize
	}
	return size
}
