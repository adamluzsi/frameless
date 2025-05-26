package crudkit

import (
	"iter"

	"go.llib.dev/frameless/pkg/iterkit"
)

func CollectQueryMany[T any](i iter.Seq2[T, error], err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	return iterkit.CollectErr(i)
}
