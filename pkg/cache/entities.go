package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/iterators"
	"strings"
	"time"
)

const ErrNotImplementedBySource errorkit.Error = "the method is not implemented by the cache source"

type EntityRepository[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.Updater[Entity]
	crud.Finder[Entity, ID]
	crud.Deleter[ID]
	FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Entity]
	Upsert(ctx context.Context, ptrs ...*Entity) error // TODO: replace Upsert with crud.Saver
}

// HitRepository is the query hit result repository.
type HitRepository[EntID any] interface {
	crud.Creator[Hit[EntID]]
	crud.Updater[Hit[EntID]]
	crud.Finder[Hit[EntID], HitID]
	crud.Deleter[HitID]
}

type (
	Hit[ID any] struct {
		QueryID   HitID `ext:"id"`
		EntityIDs []ID
		Timestamp time.Time
	}
	HitID = string
)

type Interface[Entity, ID any] interface {
	CachedQueryOne(ctx context.Context, queryKey HitID, query QueryOneFunc[Entity]) (_ent Entity, _found bool, _err error)
	CachedQueryMany(ctx context.Context, queryKey HitID, query QueryManyFunc[Entity]) iterators.Iterator[Entity]
	InvalidateCachedQuery(ctx context.Context, queryKey HitID) error
	InvalidateByID(ctx context.Context, id ID) (rErr error)
	DropCachedValues(ctx context.Context) error
}

type (
	QueryOneFunc[Entity any]  func() (ent Entity, found bool, err error)
	QueryManyFunc[Entity any] func() iterators.Iterator[Entity]
)

// QueryKey is a helper function that allows you to create QueryManyFunc Keys
type QueryKey struct {
	// ID is the unique identifier to know what query is being cached.
	// A method name or any unique name could work.
	ID string
	// ARGS contain parameters to the query that can affect the query result.
	// Supplying the ARGS ensures that a query call with different arguments cached individually.
	ARGS map[string]any

	Version int
}

func (qk QueryKey) Encode() HitID {
	var out = fmt.Sprintf("%d:%s", qk.Version, qk.ID)
	if len(qk.ARGS) == 0 {
		return out
	}
	// fmt print formatting is sorting the map content before printing,
	// which makes using the QueryKey.Encode deterministic.
	out += ":" + strings.TrimPrefix(fmt.Sprintf("%v", qk.ARGS), "map")
	return HitID(out)
}
