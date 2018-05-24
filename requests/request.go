package requests

import (
	"context"
	"io"

	"github.com/adamluzsi/frameless/dataproviders"
)

type Request interface {
	io.Closer
	Context() context.Context
	Options() dataproviders.Getter
	Data() dataproviders.Iterator
}
