package iterators

import (
	"fmt"
)

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf[T any](format string, a ...interface{}) Iterator[T] {
	return Error[T](fmt.Errorf(format, a...))
}
