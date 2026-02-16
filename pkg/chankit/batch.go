package chankit

import (
	"cmp"
	"time"

	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase/clock"
)

func Batch[T any](ch chan T, opts ...BatchOption) chan []T {
	c := option.ToConfig(opts)
	const defaultBatchSize = 64

	out := make(chan []T)
	go func() {
		defer close(out)

		// batch timeout support
		var resetTicker = func() {}
		var tickerWait = func() <-chan time.Time { return nil }
		if 0 < c.WaitLimit {
			ticker := clock.NewTicker(time.Second)
			defer ticker.Stop()
			resetTicker = func() { ticker.Reset(c.WaitLimit) }
			tickerWait = func() <-chan time.Time { return ticker.C }
		}

		// buffering
		var size = c.Size
		if size <= 0 {
			size = defaultBatchSize
		}
		var buffer []T
		var flush = func() {
			if len(buffer) == 0 {
				return
			}
			out <- buffer
			buffer = nil
		}

	collect:
		for {
			resetTicker()
			select {
			case v, ok := <-ch:
				if !ok {
					flush()
					break collect
				}
				buffer = append(buffer, v)
				if size <= len(buffer) {
					flush()
				}
			case <-tickerWait():
				flush()
			}
		}
	}()

	return out
}

type BatchOption option.Option[BatchConfig]

type BatchConfig struct {
	Size      int
	WaitLimit time.Duration
}

var _ BatchOption = BatchConfig{}

func (c BatchConfig) Configure(t *BatchConfig) {
	t.Size = cmp.Or(c.Size, t.Size)
	t.WaitLimit = cmp.Or(c.WaitLimit, t.WaitLimit)
}
