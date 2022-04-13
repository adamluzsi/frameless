package iterators

import (
	"io"

	"github.com/adamluzsi/frameless"
)

func WithCallback[T any](i frameless.Iterator[T], c Callback) frameless.Iterator[T] {
	return &CallbackIterator[T]{Iterator: i, Callback: c}
}

type Callback struct {
	OnClose func(io.Closer) error
}

type CallbackIterator[T any] struct {
	frameless.Iterator[T]
	Callback
}

func (i *CallbackIterator[T]) Close() error {
	if i.Callback.OnClose != nil {
		return i.Callback.OnClose(i.Iterator)
	}
	return i.Iterator.Close()
}
