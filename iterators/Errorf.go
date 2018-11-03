package iterators

import (
	"fmt"

	"github.com/adamluzsi/frameless"
)

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf(format string, a ...interface{}) frameless.Iterator {
	return NewError(fmt.Errorf(format, a...))
}
