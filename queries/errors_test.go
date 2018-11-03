package queries_test

import "github.com/adamluzsi/frameless/queries"

var _ error = queries.ErrNotFound
var _ error = queries.ErrIDRequired
var _ error = queries.ErrNotImplemented
