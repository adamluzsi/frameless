package crudkit

import (
	"iter"

	"go.llib.dev/frameless/pkg/iterkit"
)

func CollectQueryMany[T any](i iter.Seq2[T, error], err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	return iterkit.CollectErrIter(i)
}

func First[T any](i iter.Seq2[T, error]) (T, bool, error) {
	for v, err := range i {
		return v, err == nil, err
	}
	var zero T
	return zero, false, nil
}
