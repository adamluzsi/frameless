package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type T = frameless.T

type Storage[Ent, ID any] interface {
	CacheEntity(ctx context.Context) EntityStorage[Ent, ID]
	CacheHit(ctx context.Context) HitStorage[ID]
	frameless.OnePhaseCommitProtocol
}

type EntityStorage[Ent, ID any] interface {
	frameless.Creator[Ent]
	frameless.Updater[Ent]
	frameless.Finder[Ent, ID]
	frameless.Deleter[ID]
	FindByIDs(ctx context.Context, ids ...ID) frameless.Iterator[Ent]
	Upsert(ctx context.Context, ptrs ...*Ent) error
}

// HitStorage is the query hit result storage.
type HitStorage[EntID any] interface {
	frameless.Creator[Hit[EntID]]
	frameless.Updater[Hit[EntID]]
	frameless.Finder[Hit[EntID], string]
	frameless.Deleter[string]
}

type Hit[ID any] struct {
	QueryID   string `ext:"id"`
	EntityIDs []ID
}
