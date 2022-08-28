package iterators

import (
	"io"
)

func WithCallback[T any](i Iterator[T], c Callback) Iterator[T] {
	return &CallbackIterator[T]{Iterator: i, Callback: c}
}

type Callback struct {
	OnClose func(io.Closer) error
}

type CallbackIterator[T any] struct {
	Iterator[T]
	Callback
}

func (i *CallbackIterator[T]) Close() error {
	if i.Callback.OnClose != nil {
		return i.Callback.OnClose(i.Iterator)
	}
	return i.Iterator.Close()
}
