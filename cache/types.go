package cache

// Hit  1:N Data
// Data N:M Hit

import (
	"context"
	"github.com/adamluzsi/frameless"
)

type T = frameless.T

type Storage interface {
	CacheEntity(ctx context.Context) EntityStorage
	CacheHit(ctx context.Context) HitStorage
	frameless.OnePhaseCommitProtocol
}

type EntityStorage /* [T] */ interface {
	frameless.Creator
	frameless.Updater
	frameless.Finder
	frameless.Deleter
	FindByIDs(ctx context.Context, ids ...interface{}) frameless.Iterator /* [T] */
	Upsert(ctx context.Context, ptrs ...interface{}) error
}

// HitStorage is the query hit result storage.
type HitStorage /* Hit[T.ID] */ interface {
	frameless.Creator
	frameless.Updater
	frameless.Finder
	frameless.Deleter
}

type Hit struct {
	QueryID   string        `ext:"id"`
	EntityIDs []interface{} /* []T.ID */
}
