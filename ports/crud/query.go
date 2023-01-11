package crud

import (
	"context"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type Query[Entity any] interface {
	Query(context.Context) iterators.Iterator[Entity]
}
