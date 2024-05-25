// Package cache will supply caching solutions for your crud port compatible resources.
package cache

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/testcase/clock"
)

func New[Entity any, ID comparable](
	source Source[Entity, ID],
	cacheRepo Repository[Entity, ID],
) *Cache[Entity, ID] {
	return &Cache[Entity, ID]{
		Source:     source,
		Repository: cacheRepo,
	}
}

// Cache supplies Read/Write-Through caching to CRUD resources.
type Cache[Entity any, ID comparable] struct {
	// Source is the location of the original data
	Source Source[Entity, ID]
	// Repository is the resource that keeps the cached data.
	Repository Repository[Entity, ID]

	CachedQueryInvalidators []CachedQueryInvalidator[Entity, ID]
}

type CachedQueryInvalidator[Entity, ID any] struct {
	// CheckEntity checks an entity which is being invalidated, and using its properties,
	// you can construct the entity values
	CheckEntity func(ent Entity) []HitID
	// CheckHit meant to check a Hit to decide if it needs to be invalidated.
	// If CheckHit returns with true, then the hit will be invalidated.
	CheckHit func(hit Hit[ID]) bool
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Cache also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source[Entity, ID any] interface {
	crud.ByIDFinder[Entity, ID]
}

type Repository[Entity, ID any] interface {
	comproto.OnePhaseCommitProtocol
	Entities() EntityRepository[Entity, ID]
	Hits() HitRepository[ID]
}

// InvalidateByID will as the name suggest, invalidate an entity from the cache.
//
// If you have CachedQueryMany and CachedQueryOne usage, then you must use InvalidateCachedQuery instead of this.
// This is requires because if absence of an entity is cached in the HitRepository,
// then it is impossible to determine how to invalidate those queries using an Entity ID.
func (m *Cache[Entity, ID]) InvalidateByID(ctx context.Context, id ID) (rErr error) {
	ctx, err := m.Repository.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Repository, ctx)

	ent, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		return err
	}
	if found {
		if err := m.Repository.Entities().DeleteByID(ctx, id); err != nil {
			return err
		}
	}
	if !found {
		ent, found, err = m.Source.FindByID(ctx, id)
		if err != nil {
			return err
		}
	}
	if found {
		for _, inv := range m.CachedQueryInvalidators {
			if inv.CheckEntity == nil {
				continue
			}
			for _, hit := range inv.CheckEntity(ent) {
				if err := m.InvalidateCachedQuery(ctx, hit); err != nil {
					return err
				}
			}
		}
	}

	if err := m.InvalidateCachedQuery(ctx, m.queryKeyFindByID(id)); err != nil {
		return err
	}

	HitsThatReferenceOurEntity := iterators.Filter[Hit[ID]](m.Repository.Hits().FindAll(ctx), func(h Hit[ID]) bool {
		for _, gotID := range h.EntityIDs {
			if gotID == id {
				return true
			}
		}
		for _, inv := range m.CachedQueryInvalidators {
			if inv.CheckHit == nil {
				continue
			}
			if inv.CheckHit(h) {
				return true
			}
		}
		return false
	})

	// Invalidate related cached query hits, but not their related entities.
	// This is especially important to avoid a cascading effect that can wipe out the whole cache.
	hitIDs, err := iterators.Reduce(HitsThatReferenceOurEntity, []HitID{},
		func(ids []HitID, h Hit[ID]) []HitID {
			ids = append(ids, h.QueryID)
			return ids
		})

	if err != nil {
		return err
	}

	for _, hitID := range hitIDs {
		if _, _, err := m.invalidateCachedQueryWithoutCascadeEffect(ctx, hitID); err != nil {
			return err
		}
	}

	return nil
}

func (m *Cache[Entity, ID]) DropCachedValues(ctx context.Context) error {
	return errorkit.Merge(
		m.Repository.Hits().DeleteAll(ctx),
		m.Repository.Entities().DeleteAll(ctx))
}

func (m *Cache[Entity, ID]) InvalidateCachedQuery(ctx context.Context, queryKey HitID) (rErr error) {
	ctx, err := m.Repository.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Repository, ctx)

	// it is important to first delete the hit record to avoid a loop effect with other invalidation calls.
	hit, found, err := m.invalidateCachedQueryWithoutCascadeEffect(ctx, queryKey)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	for _, entID := range hit.EntityIDs {
		if err := m.InvalidateByID(ctx, entID); err != nil {
			return err
		}
	}

	return nil
}

func (m *Cache[Entity, ID]) invalidateCachedQueryWithoutCascadeEffect(ctx context.Context, queryKey HitID) (Hit[ID], bool, error) {
	hit, found, err := m.Repository.Hits().FindByID(ctx, queryKey)
	if err != nil || !found {
		return hit, false, err
	}
	return hit, found, m.Repository.Hits().DeleteByID(ctx, queryKey)
}

func (m *Cache[Entity, ID]) CachedQueryMany(
	ctx context.Context,
	queryKey string,
	query QueryManyFunc[Entity],
) iterators.Iterator[Entity] {
	// TODO: double check
	if ctx != nil && ctx.Err() != nil {
		return iterators.Error[Entity](ctx.Err())
	}

	hit, found, err := m.Repository.Hits().FindByID(ctx, queryKey)
	if err != nil {
		logger.Warn(ctx, fmt.Sprintf("error during retrieving hits for %s", queryKey), logger.ErrField(err))
		return query()
	}
	if found {
		// TODO: make sure that in case entity ids point to empty cache data
		//       we invalidate the hit and try again
		iter := m.Repository.Entities().FindByIDs(ctx, hit.EntityIDs...)
		if err := iter.Err(); err != nil {
			logger.Warn(ctx, "cache Repository.Entities().FindByIDs had an error", logger.ErrField(err))
			return query()
		}
		return iter
	}

	// this naive MVP approach might take a big burden on the memory.
	// If this becomes the case, it should be possible to change this into a streaming approach
	// where iterator being iterated element by element,
	// and records being created during then in the Repository
	var ids []ID
	res, err := iterators.Collect(query())
	if err != nil {
		return iterators.Error[Entity](err)
	}
	for _, v := range res {
		id, _ := extid.Lookup[ID](v)
		ids = append(ids, id)
	}

	var vs []*Entity
	for _, ent := range res {
		ent := ent // pass by value copy
		vs = append(vs, &ent)
	}

	if err := m.Repository.Entities().Upsert(ctx, vs...); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Upsert had an error", logger.ErrField(err))
		return iterators.Slice[Entity](res)
	}

	if err := m.Repository.Hits().Create(ctx, &Hit[ID]{
		QueryID:   queryKey,
		EntityIDs: ids,
		Timestamp: clock.TimeNow().UTC(),
	}); err != nil {
		logger.Warn(ctx, "cache Repository.Hits().Create had an error", logger.ErrField(err))
		return iterators.Slice[Entity](res)
	}

	return iterators.Slice[Entity](res)
}

func (m *Cache[Entity, ID]) CachedQueryOne(
	ctx context.Context,
	queryKey string,
	query QueryOneFunc[Entity],
) (_ent Entity, _found bool, _err error) {
	iter := m.CachedQueryMany(ctx, queryKey, func() iterators.Iterator[Entity] {
		ent, found, err := query()
		if err != nil {
			return iterators.Error[Entity](err)
		}
		if !found {
			return iterators.Empty[Entity]()
		}
		return iterators.Slice[Entity]([]Entity{ent})
	})

	ent, found, err := iterators.First[Entity](iter)
	if err != nil {
		return ent, false, err
	}
	if !found {
		return ent, false, nil
	}
	return ent, true, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Cache[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	source, ok := m.Source.(crud.Creator[Entity])
	if !ok {
		return fmt.Errorf("%s: %w", "Create", ErrNotImplementedBySource)
	}
	if err := source.Create(ctx, ptr); err != nil {
		return err
	}
	if err := m.Repository.Entities().Create(ctx, ptr); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Create had an error", logger.ErrField(err))
	}
	return nil
}

func (m *Cache[Entity, ID]) FindByID(ctx context.Context, id ID) (Entity, bool, error) {
	// fast path
	ent, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		logger.Warn(ctx, "cache Repository.Entities().FindByID had an error", logger.ErrField(err))
		return m.Source.FindByID(ctx, id)
	}
	if found {
		return ent, true, nil
	}
	// slow path
	return m.CachedQueryOne(ctx, m.queryKeyFindByID(id), func() (ent Entity, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	})
}

func (m *Cache[Entity, ID]) queryKeyFindByID(id ID) HitID {
	return QueryKey{
		ID:   "FindByID",
		ARGS: map[string]any{"ID": id},
	}.Encode()
}

func (m *Cache[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	source, ok := m.Source.(crud.AllFinder[Entity])
	if !ok {
		return iterators.Errorf[Entity]("%s: %w", "FindAll", ErrNotImplementedBySource)
	}
	return m.CachedQueryMany(ctx, QueryKey{ID: "FindAll"}.Encode(), func() iterators.Iterator[Entity] {
		return source.FindAll(ctx)
	})
}

func (m *Cache[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	source, ok := m.Source.(crud.Updater[Entity])
	if !ok {
		return fmt.Errorf("%s: %w", "Update", ErrNotImplementedBySource)
	}
	if err := source.Update(ctx, ptr); err != nil {
		return err
	}
	if err := m.Repository.Entities().Update(ctx, ptr); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Update had an error", logger.ErrField(err))
		if id, ok := extid.Lookup[ID](*ptr); ok {
			return m.InvalidateByID(ctx, id)
		}
	}
	return nil
}

func (m *Cache[Entity, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	source, ok := m.Source.(crud.ByIDDeleter[ID])
	if !ok {
		return fmt.Errorf("%s: %w", "DeleteByID", ErrNotImplementedBySource)
	}
	if err := source.DeleteByID(ctx, id); err != nil {
		return err
	}
	return m.InvalidateByID(ctx, id)
}

func (m *Cache[Entity, ID]) DeleteAll(ctx context.Context) error {
	source, ok := m.Source.(crud.AllDeleter)
	if !ok {
		return fmt.Errorf("%s: %w", "DeleteAll", ErrNotImplementedBySource)
	}
	if err := source.DeleteAll(ctx); err != nil {
		return err
	}
	return m.DropCachedValues(ctx)
}
