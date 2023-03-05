package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errorutil"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/iterators"
)

const ErrNotImplementedBySource errorutil.Error = "the method is not implemented by the cache source"

type EntityRepository[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.Updater[Entity]
	crud.Finder[Entity, ID]
	crud.Deleter[ID]
	FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Entity]
	Upsert(ctx context.Context, ptrs ...*Entity) error
}

// HitRepository is the query hit result repository.
type HitRepository[EntID any] interface {
	crud.Creator[Hit[EntID]]
	crud.Updater[Hit[EntID]]
	crud.Finder[Hit[EntID], string]
	crud.Deleter[string]
}

type Hit[ID any] struct {
	QueryID   string `ext:"id"`
	EntityIDs []ID
}
