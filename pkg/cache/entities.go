package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.llib.dev/frameless/internal/constant"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/iterators"
)

const ErrNotImplementedBySource errorkit.Error = "the method is not implemented by the cache source"

type EntityRepository[ENT, ID any] interface {
	crud.Creator[ENT]
	crud.Updater[ENT]
	crud.Finder[ENT, ID]
	crud.Deleter[ID]
	crud.ByIDsFinder[ENT, ID]
	crud.Saver[ENT]
}

// HitRepository is the query hit result repository.
type HitRepository[EntID any] interface {
	crud.Saver[Hit[EntID]]
	crud.Finder[Hit[EntID], HitID]
	crud.Deleter[HitID]
}

type Hit[ID any] struct {
	ID        HitID `ext:"id"`
	EntityIDs []ID
	Timestamp time.Time
}

type HitID string

type Interface[ENT, ID any] interface {
	CachedQueryOne(ctx context.Context, hid HitID, query QueryOneFunc[ENT]) (_ent ENT, _found bool, _err error)
	CachedQueryMany(ctx context.Context, hid HitID, query QueryManyFunc[ENT]) (iterators.Iterator[ENT], error)
	InvalidateCachedQuery(ctx context.Context, hid HitID) error
	InvalidateByID(ctx context.Context, id ID) (rErr error)
	DropCachedValues(ctx context.Context) error
}

type (
	QueryOneFunc[ENT any]  func(ctx context.Context) (_ ENT, found bool, _ error)
	QueryManyFunc[ENT any] func(ctx context.Context) (iterators.Iterator[ENT], error)
)

// Query is a helper that allows you to create a cache.HitID
type Query struct {
	// Name is the name of the repository's query operation.
	// A method name or any unique deterministic name is sufficient.
	Name constant.String
	// ARGS contain parameters to the query that can affect the query result.
	// Supplying the ARGS ensures that a query call with different arguments cached individually.
	// Request lifetime related values are not expected to be part of ARGS.
	// ARGS should contain values that are serializable.
	ARGS QueryARGS
	// Version can help supporting multiple version of the same cached operation,
	// so if the application rolls out with a new version, that has different behaviour or fifferend signature
	// these values can be distringuesed
	Version int
}

// QueryARGS is the argument name of the
type QueryARGS map[string]any

// String will encode the QueryKey into a string format
func (q Query) String() string {
	var id = fmt.Sprintf("%d:%s", q.Version, q.Name)
	if len(q.ARGS) == 0 {
		return id
	}
	// fmt print formatting is sorting the map content before printing,
	// which makes HitID.String deterministic.
	return id + ":" + strings.TrimPrefix(fmt.Sprintf("%v", q.ARGS), "map")
}

func (q Query) HitID() HitID {
	return HitID(q.String())
}
