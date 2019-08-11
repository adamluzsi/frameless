package iterators

import (
	"github.com/adamluzsi/frameless"
	"io"
)

func WithCallback(i frameless.Iterator, c Callback) frameless.Iterator {
	return &CallbackIterator{Iterator: i, Callback: c,}
}

type Callback struct {
	OnClose func(io.Closer) error
}

type CallbackIterator struct {
	frameless.Iterator
	Callback
}

func (i *CallbackIterator) Close() error {
	if i.Callback.OnClose != nil {
		return i.Callback.OnClose(i.Iterator)
	}
	return i.Iterator.Close()
}

//func (*CallbackIterator) Next() bool {
//	panic("implement me")
//}
//
//func (*CallbackIterator) Err() error {
//	panic("implement me")
//}
//
//func (*CallbackIterator) Decode(frameless.Entity) error {
//	panic("implement me")
//}
//
