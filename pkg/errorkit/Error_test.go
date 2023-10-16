package errorkit_test

import (
	"go.llib.dev/frameless/pkg/errorkit"
)

func ExampleError_Error() {
	const ErrSomething errorkit.Error = "something is an error"
}
