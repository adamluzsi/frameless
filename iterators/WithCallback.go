package iterators

import (
	"io"
)

func WithCallback(i Interface, c Callback) Interface {
	return &CallbackIterator{Interface: i, Callback: c}
}

type Callback struct {
	OnClose func(io.Closer) error
}

type CallbackIterator struct {
	Interface
	Callback
}

func (i *CallbackIterator) Close() error {
	if i.Callback.OnClose != nil {
		return i.Callback.OnClose(i.Interface)
	}
	return i.Interface.Close()
}
