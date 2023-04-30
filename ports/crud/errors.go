package crud

import "github.com/adamluzsi/frameless/pkg/errorkit"

const (
	ErrAlreadyExists errorkit.Error = "err-already-exists"
	ErrNotFound      errorkit.Error = "err-not-found"
)
