package iterators

import (
	"github.com/adamluzsi/frameless/reflects"
)

// NewPipe return a receiver and a sender.
// This can be used with resources that
func NewPipe() (*PipeIn, *PipeOut) {
	feed := make(chan interface{})
	done := make(chan struct{}, 1)
	err := make(chan error, 1)
	return &PipeIn{feed: feed, done: done, err: err}, &PipeOut{feed: feed, done: done, err: err}
}

// PipeOut implements iterator interface while it's still being able to receive values, used for streaming
type PipeOut struct {
	feed <-chan interface{}
	done chan<- struct{}
	err  <-chan error

	current interface{}
	lastErr error
}

// Close sends a signal back that no more value should be sent because receiver stop listening
func (i *PipeOut) Close() error {
	defer func() { recover() }()
	i.done <- struct{}{}
	close(i.done)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (i *PipeOut) Next() bool {
	e, ok := <-i.feed

	if !ok {
		return false
	}

	i.current = e
	return true
}

// Err returns an error object that the pipe sender want to present for the pipe receiver
func (i *PipeOut) Err() error {
	err, ok := <-i.err

	if ok {
		i.lastErr = err
	}

	return i.lastErr
}

// Decode will link the current buffered value to the pointer value that is given as "e"
func (i *PipeOut) Decode(ptr interface{}) error {
	return reflects.Link(i.current, ptr)
}

// PipeIn provides access to feed a pipe receiver with entities
type PipeIn struct {
	feed chan<- interface{}
	done <-chan struct{}
	err  chan<- error
}

// Encode send value to the PipeOut
// and returns ErrClosed error if no more value expected on the receiver side
func (f *PipeIn) Encode(e interface{}) error {
	select {
	case f.feed <- e:
		return nil
	case <-f.done:
		return ErrClosed
	}
}

// Error send an error object to the PipeOut side, so it will be accessible with iterator.Err()
func (f *PipeIn) Error(err error) {
	if err == nil {
		return
	}

	defer func() { recover() }()
	f.err <- err
}

// Close close the feed and err channel, which eventually notify the receiver that no more value expected
func (f *PipeIn) Close() error {
	defer func() { recover() }()
	close(f.feed)
	close(f.err)
	return nil
}
