package iterators

import (
	"github.com/adamluzsi/frameless"
)

const Break frameless.Error = `iterators:break`

func ForEach[T any](i frameless.Iterator[T], fn func(T) error) (rErr error) {
	defer func() {
		cErr := i.Close()
		if rErr == nil {
			rErr = cErr
		}
	}()
	for i.Next() {
		v := i.Value()
		err := fn(v)
		if err == Break {
			break
		}
		if err != nil {
			return err
		}
	}
	return i.Err()
}
