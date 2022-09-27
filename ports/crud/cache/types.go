package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type Storage[Ent, ID any] interface {
	CacheEntity(ctx context.Context) EntityStorage[Ent, ID]
	CacheHit(ctx context.Context) HitStorage[ID]
	comproto.OnePhaseCommitProtocol
}

type EntityStorage[Ent, ID any] interface {
	crud.Creator[Ent]
	crud.Updater[Ent]
	crud.Finder[Ent, ID]
	crud.Deleter[ID]
	FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Ent]
	Upsert(ctx context.Context, ptrs ...*Ent) error
}

// HitStorage is the query hit result storage.
type HitStorage[EntID any] interface {
	crud.Creator[Hit[EntID]]
	crud.Updater[Hit[EntID]]
	crud.Finder[Hit[EntID], string]
	crud.Deleter[string]
}

type Hit[ID any] struct {
	QueryID   string `ext:"id"`
	EntityIDs []ID
}
