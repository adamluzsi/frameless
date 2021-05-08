package frameless_test

import (
	"github.com/adamluzsi/frameless"
)

func ExampleError_Error() {
	const ErrSomething frameless.Error = "something is an error"
}
