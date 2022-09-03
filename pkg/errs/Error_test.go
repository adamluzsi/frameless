package errs_test

import (
	"github.com/adamluzsi/frameless/pkg/errs"
)

func ExampleError_Error() {
	const ErrSomething errs.Error = "something is an error"
}
