package iterators

import (
	"github.com/adamluzsi/frameless"
)

// Map allows you to do additional transformation on the values.
// This is useful in cases, where you have to alter the input value,
// or change the type all together.
// Like when you read lines from an input stream,
// and then you map the line content to a certain data structure,
// in order to not expose what steps needed in order to deserialize the input stream,
// thus protect the business rules from this information.
func Map[T any, V any](iter frameless.Iterator[T], transform MapTransformFunc[T, V]) frameless.Iterator[V] {
	return &MapIter[T, V]{Iterator: iter, Transform: transform}
}

type MapTransformFunc[T any, V any] func(T) (V, error)

type MapIter[T any, V any] struct {
	Iterator  frameless.Iterator[T]
	Transform MapTransformFunc[T, V]

	err   error
	value V
}

func (i *MapIter[T, V]) Close() error {
	return i.Iterator.Close()
}

func (i *MapIter[T, V]) Next() bool {
	if i.err != nil {
		return false
	}
	ok := i.Iterator.Next()
	if !ok {
		return false
	}
	v, err := i.Transform(i.Iterator.Value())
	if err != nil {
		i.err = err
		return false
	}
	i.value = v
	return true
}

func (i *MapIter[T, V]) Err() error {
	if i.err != nil {
		return i.err
	}
	return i.Iterator.Err()
}

func (i *MapIter[T, V]) Value() V {
	return i.value
}
