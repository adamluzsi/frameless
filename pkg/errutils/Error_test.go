package errutils_test

import (
	"github.com/adamluzsi/frameless/pkg/errutils"
)

func ExampleError_Error() {
	const ErrSomething errutils.Error = "something is an error"
}
