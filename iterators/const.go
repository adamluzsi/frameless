package iterators

import (
	"github.com/adamluzsi/frameless/consterror"
)

const (
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed consterror.Error = "Closed"
)
