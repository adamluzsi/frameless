package iterators

import (
	"sync"
	"sync/atomic"
)

// Pipe return a receiver and a sender.
// This can be used with resources that
func Pipe[T any]() (*PipeIn[T], *PipeOut[T]) {
	pipeChan := makePipeChan[T]()
	return &PipeIn[T]{pipeChan: pipeChan},
		&PipeOut[T]{pipeChan: pipeChan}
}

func makePipeChan[T any]() pipeChan[T] {
	return pipeChan[T]{
		values:    make(chan T),
		errors:    make(chan error, 1),
		outIsDone: make(chan struct{}, 1),
	}
}

type pipeChan[T any] struct {
	values    chan T
	errors    chan error
	outIsDone chan struct{}
}

// PipeOut implements iterator interface while it's still being able to receive values, used for streaming
type PipeOut[T any] struct {
	pipeChan[T]
	value T

	m sync.Mutex

	closed     int32
	nextCalled int32
	lastErr    error
}

// Close sends a signal back that no more value should be sent because receiver stops listening
func (out *PipeOut[T]) Close() error {
	defer func() { recover() }()
	atomic.CompareAndSwapInt32(&out.closed, 0, 1)
	close(out.outIsDone)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (out *PipeOut[T]) Next() bool {
	atomic.CompareAndSwapInt32(&out.nextCalled, 0, 1)
	v, ok := <-out.pipeChan.values
	if !ok {
		return false
	}
	out.value = v
	return true
}

// Err returns an error object that the pipe sender wants to present for the pipe receiver
func (out *PipeOut[T]) Err() error {
	{ // before iteration
		if atomic.LoadInt32(&out.nextCalled) == 0 {
			select {
			case _, ok := <-out.outIsDone:
				if !ok { // nothing to do, the out is already closed
					return out.getErr()
				}
			case err, ok := <-out.errors:
				if ok {
					out.setErr(err)
				}
			default:
			}
			return out.getErr()
		}
	}
	{ // after iteration
		select {
		case err, ok := <-out.errors:
			if ok {
				out.setErr(err)
			}
		case <-out.outIsDone:
		}
		return out.getErr()
	}
}

func (out *PipeOut[T]) getErr() error {
	out.m.Lock()
	defer out.m.Unlock()
	return out.lastErr
}

func (out *PipeOut[T]) setErr(err error) {
	out.m.Lock()
	defer out.m.Unlock()
	out.lastErr = err
}

// Value will link the current buffered value to the pointer value that is given as "e"
func (out *PipeOut[T]) Value() T {
	return out.value
}

// PipeIn provides access to feed a pipe receiver with entities
type PipeIn[T any] struct {
	pipeChan[T]
}

// Value sends value to the PipeOut.Value.
// It returns if sending was possible.
func (in *PipeIn[T]) Value(v T) (ok bool) {
	select {
	case in.pipeChan.values <- v:
		return true
	case <-in.pipeChan.outIsDone:
		return false
	}
}

// Error send an error object to the PipeOut side, so it will be accessible with iterator.Err()
func (in *PipeIn[T]) Error(err error) {
	if err == nil {
		return
	}

	defer func() { recover() }()
	in.pipeChan.errors <- err
}

// Close will close the feed and err channels, which eventually notify the receiver that no more value expected
func (in *PipeIn[T]) Close() error {
	defer func() { recover() }()
	close(in.pipeChan.values)
	close(in.pipeChan.errors)
	return nil
}
