package errstr_test

import "github.com/adamluzsi/frameless/errstr"

var _ error = errstr.Error("Hello, World!")
