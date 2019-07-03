package iterators

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

// NewPipe return a receiver and a sender.
// This can be used with resources that
func NewPipe() (*PipeReceiver, *PipeSender) {
	feed := make(chan frameless.Entity)
	done := make(chan struct{}, 1)
	err := make(chan error, 1)
	return &PipeReceiver{feed: feed, done: done, err: err},
		&PipeSender{feed: feed, done: done, err: err}
}

// PipeReceiver implements iterator interface while it's still being able to receive values, used for streaming
type PipeReceiver struct {
	feed <-chan frameless.Entity
	done chan<- struct{}
	err  <-chan error

	current frameless.Entity
	lastErr error
}

// Close sends a signal back that no more value should be sent because receiver stop listening
func (i *PipeReceiver) Close() error {
	defer func() { recover() }()
	i.done <- struct{}{}
	close(i.done)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (i *PipeReceiver) Next() bool {
	e, ok := <-i.feed

	if !ok {
		return false
	}

	i.current = e
	return true
}

// Err returns an error object that the pipe sender want to present for the pipe receiver
func (i *PipeReceiver) Err() error {
	err, ok := <-i.err

	if ok {
		i.lastErr = err
	}

	return i.lastErr
}

// Decode will link the current buffered value to the pointer value that is given as "e"
func (i *PipeReceiver) Decode(e interface{}) error {
	return reflects.Link(i.current, e)
}

// PipeSender provides access to feed a pipe receiver with entities
type PipeSender struct {
	feed chan<- frameless.Entity
	done <-chan struct{}
	err  chan<- error
}

// Encode send value to the PipeReceiver
// and returns ErrClosed error if no more value expected on the receiver side
func (f *PipeSender) Encode(e frameless.Entity) error {
	select {
	case f.feed <- e:
		return nil
	case <-f.done:
		return ErrClosed
	}
}

// Error send an error object to the PipeReceiver side, so it will be accessible with iterator.Err()
func (f *PipeSender) Error(err error) {
	if err == nil {
		return
	}

	defer func() { recover() }()
	f.err <- err
}

// Close close the feed and err channel, which eventually notify the receiver that no more value expected
func (f *PipeSender) Close() error {
	defer func() { recover() }()
	close(f.feed)
	close(f.err)
	return nil
}
