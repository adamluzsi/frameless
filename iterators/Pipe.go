package iterators

// Pipe return a receiver and a sender.
// This can be used with resources that
func Pipe[T any]() (*PipeIn[T], *PipeOut[T]) {
	valueChan := make(chan T)
	doneChan := make(chan struct{}, 1)
	errChan := make(chan error, 1)
	return &PipeIn[T]{ValueChan: valueChan, DoneChan: doneChan, ErrChan: errChan},
		&PipeOut[T]{ValueChan: valueChan, DoneChan: doneChan, ErrChan: errChan}
}

// PipeOut implements iterator interface while it's still being able to receive values, used for streaming
type PipeOut[T any] struct {
	ValueChan <-chan T
	DoneChan  chan<- struct{}
	ErrChan   <-chan error

	value   T
	lastErr error
}

// Close sends a signal back that no more value should be sent because receiver stop listening
func (i *PipeOut[T]) Close() error {
	defer func() { recover() }()
	i.DoneChan <- struct{}{}
	close(i.DoneChan)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (i *PipeOut[T]) Next() bool {
	v, ok := <-i.ValueChan
	if !ok {
		return false
	}

	i.value = v
	return true
}

// Err returns an error object that the pipe sender want to present for the pipe receiver
func (i *PipeOut[T]) Err() error {
	err, ok := <-i.ErrChan
	if ok {
		i.lastErr = err
	}

	return i.lastErr
}

// Decode will link the current buffered value to the pointer value that is given as "e"
func (i *PipeOut[T]) Value() T {
	return i.value
}

// PipeIn provides access to feed a pipe receiver with entities
type PipeIn[T any] struct {
	ValueChan chan<- T
	DoneChan  <-chan struct{}
	ErrChan   chan<- error
}

// Encode send value to the PipeOut
// and returns ErrClosed error if no more value expected on the receiver side
func (f *PipeIn[T]) Value(v T) (ok bool) {
	select {
	case f.ValueChan <- v:
		return true
	case <-f.DoneChan:
		return false
	}
}

// ErrorIter send an error object to the PipeOut side, so it will be accessible with iterator.Err()
func (f *PipeIn[T]) Error(err error) {
	if err == nil {
		return
	}

	defer func() { recover() }()
	f.ErrChan <- err
}

// Close close the feed and err channel, which eventually notify the receiver that no more value expected
func (f *PipeIn[T]) Close() error {
	defer func() { recover() }()
	close(f.ValueChan)
	close(f.ErrChan)
	return nil
}
