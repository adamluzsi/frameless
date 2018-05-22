package request

import (
	"context"
	"io"

	"github.com/adamluzsi/frameless/dataprovider"
)

type Request interface {
	io.Closer
	Context() context.Context
	Options() dataprovider.Getter
	Data() dataprovider.Iterator
}
