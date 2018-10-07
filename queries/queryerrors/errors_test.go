package queryerrors_test

import "github.com/adamluzsi/frameless/queries/queryerrors"

var _ error = queryerrors.ErrNotFound
var _ error = queryerrors.ErrIDRequired
