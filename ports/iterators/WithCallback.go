package iterators

import "go.llib.dev/frameless/pkg/errorkit"

func OnClose(fn func() error) CallbackOption {
	return callbackFunc(func(c *callbackConfig) {
		c.OnClose = append(c.OnClose, fn)
	})
}

func WithCallback[T any](i Iterator[T], cs ...CallbackOption) Iterator[T] {
	if len(cs) == 0 {
		return i
	}
	return &callbackIterator[T]{Iterator: i, CallbackConfig: toCallback(cs)}
}

type callbackIterator[T any] struct {
	Iterator[T]
	CallbackConfig callbackConfig
}

func (i *callbackIterator[T]) Close() error {
	var errs []error
	errs = []error{i.Iterator.Close()}
	for _, onClose := range i.CallbackConfig.OnClose {
		errs = append(errs, onClose())
	}
	return errorkit.Merge(errs...)
}

func toCallback(cs []CallbackOption) callbackConfig {
	var c callbackConfig
	for _, opt := range cs {
		opt.configure(&c)
	}
	return c
}

type callbackConfig struct {
	OnClose []func() error
}

type CallbackOption interface {
	configure(c *callbackConfig)
}

type callbackFunc func(c *callbackConfig)

func (fn callbackFunc) configure(c *callbackConfig) { fn(c) }
