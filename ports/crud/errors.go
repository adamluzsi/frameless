package crud

import "go.llib.dev/frameless/pkg/errorkit"

const (
	ErrAlreadyExists errorkit.Error = "err-already-exists"
	ErrNotFound      errorkit.Error = "err-not-found"
)
