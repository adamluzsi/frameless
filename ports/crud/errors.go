package crud

import "github.com/adamluzsi/frameless/pkg/errorutil"

const (
	ErrAlreadyExists errorutil.Error = "err-already-exists"
	ErrNotFound      errorutil.Error = "err-not-found"
)
