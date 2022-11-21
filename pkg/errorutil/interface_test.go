package errorutil

import "github.com/adamluzsi/frameless/pkg/errorutil/internal"

var (
	_ internal.ErrorAs     = multiError{}
	_ internal.ErrorIs     = multiError{}
	_ internal.ErrorUnwrap = &tagError{}
	_ internal.ErrorUnwrap = errorWithContext{}
	_ internal.ErrorUnwrap = errorWithDetail{}
)
