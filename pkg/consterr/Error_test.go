package consterr_test

import (
	"github.com/adamluzsi/frameless/pkg/consterr"
)

func ExampleError_Error() {
	const ErrSomething consterr.Error = "something is an error"
}
