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
	value    T
	iterated bool
	lastErr  error
}

// Close sends a signal back that no more value should be sent because receiver stops listening
func (out *PipeOut[T]) Close() error {
	defer func() { recover() }()
	close(out.outIsDone)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (out *PipeOut[T]) Next() bool {
	out.iterated = true
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
		if !out.iterated {
			select {
			case _, ok := <-out.outIsDone:
				if !ok { // nothing to do, the out is already closed
					return out.lastErr
				}
			case err, ok := <-out.errors:
				if ok {
					out.lastErr = err
				}
			default:
			}
			return out.lastErr
		}
	}
	{ // after iteration
		select {
		case err, ok := <-out.errors:
			if ok {
				out.lastErr = err
			}
		case <-out.outIsDone:
		}
		return out.lastErr
	}
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
