package iterators

import (
	"io"
)

func WithCallback(i Iterator, c Callback) Iterator {
	return &CallbackIterator{Iterator: i, Callback: c}
}

type Callback struct {
	OnClose func(io.Closer) error
}

type CallbackIterator struct {
	Iterator
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
//func (*CallbackIterator) Decode(interface{}) error {
//	panic("implement me")
//}
//
