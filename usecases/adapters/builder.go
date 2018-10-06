package adapters

import (
	"io"

	"github.com/adamluzsi/frameless"
)

// PresenterBuilder is an example how presenter should be created
type PresenterBuilder func(io.Writer) frameless.Encoder
