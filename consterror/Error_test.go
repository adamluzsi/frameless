package consterror_test

import "github.com/adamluzsi/frameless/consterror"

func ExampleError_Error() {
	const ErrSomething consterror.Error = "something is an error"
}
