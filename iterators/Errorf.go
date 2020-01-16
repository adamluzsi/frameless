package iterators

import (
	"fmt"
)

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf(format string, a ...interface{}) Interface {
	return NewError(fmt.Errorf(format, a...))
}
