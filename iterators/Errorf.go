package iterators

import (
	"fmt"

	"github.com/adamluzsi/frameless"
)

func Errorf(format string, a ...interface{}) frameless.Iterator {
	return NewError(fmt.Errorf(format, a...))
}
