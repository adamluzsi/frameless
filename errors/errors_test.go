package errors_test

import (
	"github.com/adamluzsi/frameless/errors"
)

var _ error = errors.ErrNotFound
var _ error = errors.ErrIDRequired
var _ error = errors.ErrNotImplemented
