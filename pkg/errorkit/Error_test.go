package errorkit_test

import (
	"github.com/adamluzsi/frameless/pkg/errorkit"
)

func ExampleError_Error() {
	const ErrSomething errorkit.Error = "something is an error"
}
