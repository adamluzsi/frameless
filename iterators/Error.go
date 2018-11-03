package iterators

// NewError returns an Iterator that only can do is returning an Err and never have next element
func NewError(err error) *Error {
	return &Error{err}
}

// Error iterator can be used for returning an error wrapped with iterator interface.
// This can be used when external resource encounter unexpected non recoverable error during query execution.
type Error struct {
	err error
}

func (i *Error) Close() error {
	return nil
}

func (i *Error) Next() bool {
	return false
}

func (i *Error) Err() error {
	return i.err
}

func (i *Error) Decode(interface{}) error {
	return nil
}
