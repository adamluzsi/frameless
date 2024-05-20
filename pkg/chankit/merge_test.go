package chankit_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/chankit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleMerge() {
	var (
		ch1 = make(chan int)
		ch2 = make(chan int)
		ch3 = make(chan int)
	)

	out, cancel := chankit.Merge(ch1, ch2, ch3)
	defer cancel()

	// out will receive values from ch1, ch2, ch3
	<-out
}

func TestMerge(t *testing.T) {
	t.Run("no channel is given", func(t *testing.T) {
		ch, cancel := chankit.Merge[any, chan any]()
		assert.NotNil(t, cancel, "a valid cancel function is expected to received back")
		assert.Nil(t, ch, "nil channel is expected, as it would not cause deadlock for those who start to listen to it")
		assert.NotPanic(t, cancel)
	})
	t.Run("1 channel is given", func(t *testing.T) {
		in := make(chan int)
		out, cancel := chankit.Merge(in)
		assert.Within(t, time.Second, func(context.Context) {
			exp := rnd.Int()
			go func() { in <- exp }()
			assert.Equal(t, exp, <-out)
		})
		assert.NotPanic(t, cancel)
	})

	t.Run("n channel is given", func(t *testing.T) {
		var channels []chan int

		rnd.Repeat(3, 20, func() {
			channels = append(channels, make(chan int))
		})

		out, cancel := chankit.Merge(channels...)

		for _, in := range channels {
			exp := rnd.Int()
			assert.Within(t, time.Second, func(context.Context) {
				go func() {
					in <- exp
					in <- exp + 1
				}()
				assert.Equal(t, exp, <-out)
				assert.Equal(t, exp+1, <-out)
			})
		}

		assert.NotPanic(t, cancel)
	})

	t.Run("2", func(t *testing.T) {
		var channels []chan int
		for i := 0; i < 11; i++ {
			channels = append(channels, make(chan int))
		}

		out, cancel := chankit.Merge(channels...)

		for _, in := range channels {
			exp := rnd.Int()
			assert.Within(t, time.Second, func(context.Context) {
				go func() {
					in <- exp
					in <- exp + 1
				}()
				assert.Equal(t, exp, <-out)
				assert.Equal(t, exp+1, <-out)
			})
		}

		assert.NotPanic(t, cancel)
	})

	t.Run("optimised", func(t *testing.T) {
		for n := 0; n < 20; n++ {
			count := n + 1

			t.Run(fmt.Sprintf("%d", count), func(t *testing.T) {
				var channels []chan int
				for i := 0; i < count; i++ {
					channels = append(channels, make(chan int))
				}

				out, cancel := chankit.Merge(channels...)
				defer cancel()

				for _, in := range channels {
					exp := rnd.Int()
					go func() {
						in <- exp
						in <- exp + 1
					}()
					assert.Within(t, time.Second, func(context.Context) {
						assert.Equal(t, exp, <-out)
						assert.Equal(t, exp+1, <-out)
					})
				}
			})
		}
	})

	t.Run("100+", func(t *testing.T) {
		var channels []chan int

		rnd.Repeat(101, 150, func() {
			channels = append(channels, make(chan int))
		})

		out, cancel := chankit.Merge(channels...)

		for _, in := range channels {
			exp := rnd.Int()
			assert.Within(t, time.Second, func(context.Context) {
				go func() { in <- exp }()
				assert.Equal(t, exp, <-out)
			})
		}

		assert.NotPanic(t, cancel)
	})

	t.Run("cancel", func(t *testing.T) {
		var channels []chan int
		rnd.Repeat(101, 150, func() {
			channels = append(channels, make(chan int))
		})

		var (
			ch   = random.Pick(rnd, channels...)
			done = make(chan struct{})
		)
		defer close(done)

		go func() {
			for {
				select {
				case ch <- 42:
				case <-done:
					return
				}
			}
		}()

		_, cancel := chankit.Merge(channels...)

		assert.Within(t, time.Second, func(ctx context.Context) { cancel() })
	})
}

func TestMerge_spike(t *testing.T) {
	if _, ok := os.LookupEnv("SPIKE"); !ok {
		t.Skip()
	}

	ch, cancel := chankit.Merge(make(chan int))
	defer cancel()

	select {
	case <-ch: // check htop for CPU usage
	case <-time.After(15 * time.Second):
	}
}

func BenchmarkMerge(b *testing.B) {
	b.Run("5 - chankit.Merge", benchMerge(5, chankit.Merge[int, chan int]))
	b.Run("5 - reflect Merge", benchMerge(5, reflectMerge[int]))

	b.Run("10 - chankit.Merge", benchMerge(10, chankit.Merge[int, chan int]))
	b.Run("10 - reflect Merge", benchMerge(10, reflectMerge[int]))

	b.Run("25 - chankit.Merge", benchMerge(25, chankit.Merge[int, chan int]))
	b.Run("25 - reflect Merge", benchMerge(25, reflectMerge[int]))

	b.Run("100 - chankit.Merge", benchMerge(100, chankit.Merge[int, chan int]))
	b.Run("100 - reflect Merge", benchMerge(100, reflectMerge[int]))
}

func benchMerge(chanCount int, subject func(...chan int) (<-chan int, func())) func(b *testing.B) {
	return func(b *testing.B) {
		channels, inChanCancel := makeBenchChannels(b, chanCount)
		defer inChanCancel()

		out, cancel := subject(channels...)
		defer cancel()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			<-out
		}
		b.StopTimer()
	}
}

func makeBenchChannels(tb testing.TB, count int) ([]chan int, func()) {
	tb.Helper()
	var (
		channels []chan int
		wg       sync.WaitGroup
	)
	for i := 0; i < count; i++ {
		channels = append(channels, make(chan int))
	}
	var done = make(chan struct{})
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000000; i++ {
				select {
				case <-done:
					return
				case channels[len(channels)-1] <- i:
				}
			}
		}()
	}
	var onClose sync.Once
	return channels, func() {
		onClose.Do(func() {
			close(done)
			wg.Wait()
			for _, ch := range channels {
				close(ch)
			}
		})
	}
}

func reflectMerge[T any](channels ...chan T) (<-chan T, func()) {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		defer close(out)
		var cases []reflect.SelectCase
		for _, ch := range channels {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(ch),
			})
		}
		for len(cases) > 0 {
			chosen, value, ok := reflect.Select(cases)
			if !ok {
				// If the channel is closed, it removes the closed channel from the cases slice.
				// It does this by creating a new slice containing all elements before the chosen index (cases[:chosen])
				// and all elements after the chosen index (cases[chosen+1:]), effectively excluding the closed channel.
				// The ellipsis (...) after cases[chosen+1:] unpacks the elements of the slice for appending.
				cases = append(cases[:chosen], cases[chosen+1:]...)
				continue
			}

			select {
			case <-done:
				return
			case out <- value.Interface().(T):
			}
		}
	}()
	var once sync.Once
	return out, func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}
