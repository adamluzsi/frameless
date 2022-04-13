package iterators

import (
	"fmt"

	"github.com/adamluzsi/frameless"
)

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf[T any](format string, a ...interface{}) frameless.Iterator[T] {
	return NewError[T](fmt.Errorf(format, a...))
}
