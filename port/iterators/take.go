package iterators

import (
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
)

// Head takes the first n element, similarly how the coreutils "head" app works.
func Head[T any](iter Iterator[T], n int) Iterator[T] {
	var (
		index     int
		closeOnce sync.Once
		closeErr  error
	)
	var close = func() error {
		closeOnce.Do(func() {
			closeErr = iter.Close()
		})
		return closeErr
	}
	return Func[T](func() (v T, ok bool, err error) {
		if n <= index {
			return v, false, errorkit.Merge(close(), iter.Err())
		}
		hasNext := iter.Next()
		if !hasNext {
			return v, false, iter.Err()
		}
		defer func() { index++ }()
		return iter.Value(), hasNext, nil
	}, OnClose(close))
}
