package chankit

import (
	"sync"

	"go.llib.dev/frameless/pkg/slicekit"
)

// Merge will join channels into a single one channel.
// If any of the channels receive a value, it will surface from out.
func Merge[T any, Chan chan T | <-chan T](chans ...Chan) (_ <-chan T, cancel func()) {
	switch len(chans) {
	case 0:
		return nil, func() {}

	case 1:
		return chans[0], func() {}

	case 2, 3, 4, 5, 6, 7, 8, 9, 10:
		var (
			out  = make(chan T)
			done = make(chan struct{})
			wg   sync.WaitGroup
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			optimisedMerge[T, Chan](done, out, chans)
		}()
		var onClose sync.Once
		return out, func() {
			onClose.Do(func() { close(done) })
			wg.Wait()
		}

	default:
		return splitMerge[T, Chan](chans)
	}
}

func optimisedMerge[T any, Chan chan T | <-chan T](
	done chan struct{},
	out chan<- T,
	chans []Chan,
) {
	var handle = func(v T, ok bool) {
		if !ok {
			return
		}
		select {
		case <-done:
			return
		case out <- v:
		}
	}
	switch len(chans) {
	case 0:
		return

	case 1:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			}
		}

	case 2:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			}
		}

	case 3:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			}
		}

	case 4:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			}
		}

	case 5:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			}
		}

	case 6:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			case v, ok := <-chans[5]:
				handle(v, ok)
			}
		}

	case 7:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			case v, ok := <-chans[5]:
				handle(v, ok)
			case v, ok := <-chans[6]:
				handle(v, ok)
			}
		}

	case 8:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			case v, ok := <-chans[5]:
				handle(v, ok)
			case v, ok := <-chans[6]:
				handle(v, ok)
			case v, ok := <-chans[7]:
				handle(v, ok)
			}
		}

	case 9:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			case v, ok := <-chans[5]:
				handle(v, ok)
			case v, ok := <-chans[6]:
				handle(v, ok)
			case v, ok := <-chans[7]:
				handle(v, ok)
			case v, ok := <-chans[8]:
				handle(v, ok)
			}
		}

	case 10:
		for {
			select {
			case <-done:
				return
			case v, ok := <-chans[0]:
				handle(v, ok)
			case v, ok := <-chans[1]:
				handle(v, ok)
			case v, ok := <-chans[2]:
				handle(v, ok)
			case v, ok := <-chans[3]:
				handle(v, ok)
			case v, ok := <-chans[4]:
				handle(v, ok)
			case v, ok := <-chans[5]:
				handle(v, ok)
			case v, ok := <-chans[6]:
				handle(v, ok)
			case v, ok := <-chans[7]:
				handle(v, ok)
			case v, ok := <-chans[8]:
				handle(v, ok)
			case v, ok := <-chans[9]:
				handle(v, ok)
			}
		}

	default:
		ch, cancel := splitMerge[T, Chan](chans)
		defer cancel()
		for {
			select {
			case <-done:
				return
			case v, ok := <-ch:
				handle(v, ok)
			}
		}
	}
}

func splitMerge[T any, Chan chan T | <-chan T](chans []Chan) (<-chan T, func()) {
	var (
		merged  []<-chan T
		cancels []func()
	)
	for _, chs := range slicekit.Batch(chans, 10) {
		ch, ccl := Merge[T](chs...)
		merged = append(merged, ch)
		cancels = append(cancels, ccl)
	}
	out, outCancel := Merge[T](merged...)
	cancels = append(cancels, outCancel)

	return out, func() {
		for _, cl := range cancels {
			defer cl()
		}
	}
}
