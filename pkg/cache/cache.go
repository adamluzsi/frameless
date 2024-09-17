// Package cache will supply caching solutions for your crud port compatible resources.
package cache

import (
	"context"
	"errors"
	"fmt"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/testcase/clock"
)

func New[ENT any, ID comparable](
	source Source[ENT, ID],
	cacheRepo Repository[ENT, ID],
) *Cache[ENT, ID] {
	return &Cache[ENT, ID]{
		Source:     source,
		Repository: cacheRepo,
	}
}

// Cache supplies Read/Write-Through caching to CRUD resources.
type Cache[ENT any, ID comparable] struct {
	// Source is the location of the original data
	Source Source[ENT, ID]
	// Repository is the resource that keeps the cached data.
	Repository Repository[ENT, ID]
	// IDA [optional] is the ENT's ID accessor.
	//
	// default: extid Lookup/Set
	IDA extid.Accessor[ENT, ID]

	CachedQueryInvalidators []CachedQueryInvalidator[ENT, ID]
}

type CachedQueryInvalidator[ENT, ID any] struct {
	// CheckEntity checks an entity which is being invalidated, and using its properties,
	// you can construct the entity values
	CheckEntity func(ent ENT) []HitID
	// CheckHit meant to check a Hit to decide if it needs to be invalidated.
	// If CheckHit returns with true, then the hit will be invalidated.
	CheckHit func(hit Hit[ID]) bool
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Cache also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source[ENT, ID any] interface {
	crud.ByIDFinder[ENT, ID]
}

type Repository[ENT, ID any] interface {
	comproto.OnePhaseCommitProtocol
	Entities() EntityRepository[ENT, ID]
	Hits() HitRepository[ID]
}

// InvalidateByID will as the name suggest, invalidate an entity from the cache.
//
// If you have CachedQueryMany and CachedQueryOne usage, then you must use InvalidateCachedQuery instead of this.
// This is requires because if absence of an entity is cached in the HitRepository,
// then it is impossible to determine how to invalidate those queries using an ENT ID.
func (m *Cache[ENT, ID]) InvalidateByID(ctx context.Context, id ID) (rErr error) {
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

func (m *Cache[ENT, ID]) DropCachedValues(ctx context.Context) error {
	return errorkit.Merge(
		m.Repository.Hits().DeleteAll(ctx),
		m.Repository.Entities().DeleteAll(ctx))
}

func (m *Cache[ENT, ID]) InvalidateCachedQuery(ctx context.Context, queryKey HitID) (rErr error) {
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

func (m *Cache[ENT, ID]) invalidateCachedQueryWithoutCascadeEffect(ctx context.Context, queryKey HitID) (Hit[ID], bool, error) {
	hit, found, err := m.Repository.Hits().FindByID(ctx, queryKey)
	if err != nil || !found {
		return hit, false, err
	}
	return hit, found, m.Repository.Hits().DeleteByID(ctx, queryKey)
}

func (m *Cache[ENT, ID]) CachedQueryMany(
	ctx context.Context,
	queryKey string,
	query QueryManyFunc[ENT],
) iterators.Iterator[ENT] {
	// TODO: double check
	if ctx != nil && ctx.Err() != nil {
		return iterators.Error[ENT](ctx.Err())
	}

	hit, found, err := m.Repository.Hits().FindByID(ctx, queryKey)
	if err != nil {
		logger.Warn(ctx, fmt.Sprintf("error during retrieving hits for %s", queryKey), logging.ErrField(err))
		return query()
	}
	if found {
		if len(hit.EntityIDs) == 0 {
			return iterators.Empty[ENT]()
		}
		iter := m.Repository.Entities().FindByIDs(ctx, hit.EntityIDs...)
		if err := iter.Err(); err != nil {
			logger.Warn(ctx, "cache Repository.Entities().FindByIDs had an error", logging.ErrField(err))
			if errors.Is(err, crud.ErrNotFound) {
				_ = m.Repository.Hits().DeleteByID(ctx, hit.QueryID)
			}
			return query()
		}
		return iter
	}

	// this naive MVP approach might take a big burden on the memory.
	// If this becomes the case, it should be possible to change this into a streaming approach
	// where iterator being iterated element by element,
	// and records being created during then in the Repository
	res, err := iterators.Collect(query())
	if err != nil {
		return iterators.Error[ENT](err)
	}
	var ids []ID
	for _, v := range res {
		id, _ := m.IDA.Lookup(v)
		ids = append(ids, id)
	}

	var vs []*ENT
	for _, ent := range res {
		ent := ent // pass by value copy
		vs = append(vs, &ent)
	}

	for _, ptr := range vs {
		if err := m.Repository.Entities().Save(ctx, ptr); err != nil {
			logger.Warn(ctx, "cache Repository.Entities().Save had an error", logging.ErrField(err))
			return iterators.Slice[ENT](res)
		}
	}

	if err := m.Repository.Hits().Save(ctx, &Hit[ID]{
		QueryID:   queryKey,
		EntityIDs: ids,
		Timestamp: clock.Now().UTC(),
	}); err != nil {
		logger.Warn(ctx, "cache Repository.Hits().Save had an error", logging.ErrField(err))
		return iterators.Slice[ENT](res)
	}

	return iterators.Slice[ENT](res)
}

func (m *Cache[ENT, ID]) CachedQueryOne(
	ctx context.Context,
	queryKey string,
	query QueryOneFunc[ENT],
) (_ent ENT, _found bool, _err error) {
	iter := m.CachedQueryMany(ctx, queryKey, func() iterators.Iterator[ENT] {
		ent, found, err := query()
		if err != nil {
			return iterators.Error[ENT](err)
		}
		if !found {
			return iterators.Empty[ENT]()
		}
		return iterators.Slice[ENT]([]ENT{ent})
	})
	ent, found, err := iterators.First[ENT](iter)
	if err != nil {
		return ent, false, err
	}
	if !found {
		return ent, false, nil
	}
	return ent, true, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Cache[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	source, ok := m.Source.(crud.Creator[ENT])
	if !ok {
		return fmt.Errorf("%s: %w", "Create", ErrNotImplementedBySource)
	}
	if err := source.Create(ctx, ptr); err != nil {
		return err
	}
	if err := m.Repository.Entities().Create(ctx, ptr); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Create had an error", logging.ErrField(err))
	}
	return nil
}

func (m *Cache[ENT, ID]) FindByID(ctx context.Context, id ID) (ENT, bool, error) {
	// fast path
	ent, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		logger.Warn(ctx, "cache Repository.Entities().FindByID had an error", logging.ErrField(err))
		return m.Source.FindByID(ctx, id)
	}
	if found {
		return ent, true, nil
	}
	// slow path
	return m.CachedQueryOne(ctx, m.queryKeyFindByID(id), func() (ent ENT, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	})
}

func (m *Cache[ENT, ID]) queryKeyFindByID(id ID) HitID {
	return QueryKey{
		ID:   "FindByID",
		ARGS: map[string]any{"ID": id},
	}.Encode()
}

func (m *Cache[ENT, ID]) FindAll(ctx context.Context) iterators.Iterator[ENT] {
	source, ok := m.Source.(crud.AllFinder[ENT])
	if !ok {
		return iterators.Errorf[ENT]("%s: %w", "FindAll", ErrNotImplementedBySource)
	}
	return m.CachedQueryMany(ctx, QueryKey{ID: "FindAll"}.Encode(), func() iterators.Iterator[ENT] {
		return source.FindAll(ctx)
	})
}

func (m *Cache[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	source, ok := m.Source.(crud.Updater[ENT])
	if !ok {
		return fmt.Errorf("%s: %w", "Update", ErrNotImplementedBySource)
	}
	if err := source.Update(ctx, ptr); err != nil {
		return err
	}
	if err := m.Repository.Entities().Update(ctx, ptr); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Update had an error", logging.ErrField(err))
		if id, ok := m.IDA.Lookup(*ptr); ok {
			return m.InvalidateByID(ctx, id)
		}
	}
	return nil
}

func (m *Cache[ENT, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	source, ok := m.Source.(crud.ByIDDeleter[ID])
	if !ok {
		return fmt.Errorf("%s: %w", "DeleteByID", ErrNotImplementedBySource)
	}
	if err := source.DeleteByID(ctx, id); err != nil {
		return err
	}
	return m.InvalidateByID(ctx, id)
}

func (m *Cache[ENT, ID]) DeleteAll(ctx context.Context) error {
	source, ok := m.Source.(crud.AllDeleter)
	if !ok {
		return fmt.Errorf("%s: %w", "DeleteAll", ErrNotImplementedBySource)
	}
	if err := source.DeleteAll(ctx); err != nil {
		return err
	}
	return m.DropCachedValues(ctx)
}
