package iterators

// Map allows you to do additional transformation on the values.
// This is useful in cases, where you have to alter the input value,
// or change the type all together.
// Like when you read lines from an input stream,
// and then you map the line content to a certain data structure,
// in order to not expose what steps needed in order to deserialize the input stream,
// thus protect the business rules from this information.
func Map[To any, From any](iter Iterator[From], transform func(From) (To, error)) Iterator[To] {
	return &mapIter[From, To]{
		Iterator:  iter,
		Transform: transform,
	}
}

type mapIter[From any, To any] struct {
	Iterator  Iterator[From]
	Transform func(From) (To, error)

	err   error
	value To
}

func (i *mapIter[From, To]) Close() error {
	return i.Iterator.Close()
}

func (i *mapIter[From, To]) Next() bool {
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

func (i *mapIter[From, To]) Err() error {
	if i.err != nil {
		return i.err
	}
	return i.Iterator.Err()
}

func (i *mapIter[From, To]) Value() To {
	return i.value
}
