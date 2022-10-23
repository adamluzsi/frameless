package errorutil_test

import (
	"github.com/adamluzsi/frameless/pkg/errorutil"
)

func ExampleError_Error() {
	const ErrSomething errorutil.Error = "something is an error"
}
