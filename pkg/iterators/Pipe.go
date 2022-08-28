package iterators

// Pipe return a receiver and a sender.
// This can be used with resources that
func Pipe[T any]() (*PipeIn[T], *PipeOut[T]) {
	pipeChan := makePipeChan[T]()
	return &PipeIn[T]{pipeChan: pipeChan},
		&PipeOut[T]{pipeChan: pipeChan}
}

func makePipeChan[T any]() pipeChan[T] {
	return pipeChan[T]{
		values: make(chan T),
		done:   make(chan struct{}, 1),
		err:    make(chan error, 1),
	}
}

type pipeChan[T any] struct {
	values chan T
	done   chan struct{}
	err    chan error
}

// PipeOut implements iterator interface while it's still being able to receive values, used for streaming
type PipeOut[T any] struct {
	pipeChan[T]
	value   T
	lastErr error
}

// Close sends a signal back that no more value should be sent because receiver stop listening
func (i *PipeOut[T]) Close() error {
	defer func() { recover() }()
	i.done <- struct{}{}
	close(i.done)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (i *PipeOut[T]) Next() bool {
	v, ok := <-i.pipeChan.values
	if !ok {
		return false
	}

	i.value = v
	return true
}

// Err returns an error object that the pipe sender want to present for the pipe receiver
func (i *PipeOut[T]) Err() error {
	err, ok := <-i.err
	if ok {
		i.lastErr = err
	}

	return i.lastErr
}

// Value will link the current buffered value to the pointer value that is given as "e"
func (i *PipeOut[T]) Value() T {
	return i.value
}

// PipeIn provides access to feed a pipe receiver with entities
type PipeIn[T any] struct {
	pipeChan[T]
}

// Value send value to the PipeOut.Value.
// It returns if sending was possible.
func (f *PipeIn[T]) Value(v T) (ok bool) {
	select {
	case f.pipeChan.values <- v:
		return true
	case <-f.pipeChan.done:
		return false
	}
}

// Error send an error object to the PipeOut side, so it will be accessible with iterator.Err()
func (f *PipeIn[T]) Error(err error) {
	if err == nil {
		return
	}

	defer func() { recover() }()
	f.pipeChan.err <- err
}

// Close will close the feed and err channels, which eventually notify the receiver that no more value expected
func (f *PipeIn[T]) Close() error {
	defer func() { recover() }()
	close(f.pipeChan.values)
	close(f.pipeChan.err)
	return nil
}
