// Package cache will supply caching solutions for your crud port compatible resources.
package cache

import (
	"context"
	"errors"
	"fmt"

	"go.llib.dev/frameless/pkg/cache/internal/memory"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/guard"
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
	// Invalidators [optional] is a list of invalidation rule which is being used whenever an entity is being invalidated.
	// It is ideal to invalidate a query by reconstucting the cache.HitID using the contents of the ENT OR ID.
	Invalidators []Invalidator[ENT, ID]
	// RefreshBehind [optional] enables background refreshing of cache.
	// When set to true, after the cache serves stale data, it triggers an
	// asynchronous update from the data source to refresh the cache in the background.
	// This ensures eventual consistency without blocking read operations.
	//
	// If you want to ensure that Refresh-Behind queries don't overlap between parallel cache instances,
	// then make sure that Scheduler uses a distributed locking.
	//
	// default: false
	RefreshBehind bool
	// Locks is used to synch background task scheduling.
	// RefreshBehind depends on Locks.
	//
	// default: application level Locks
	Locks Locks
	// TimeToLive defines the lifespan of cached data.
	// Cached entries older than this duration are considered stale and will be
	// either refreshed or invalidated on the next access, depending on the cache policy.
	// A zero value means no expiration (cache entries never expire by age).
	// TimeToLive time.Duration

	jobs tasker.JobGroup[tasker.FireAndForget]
}

type Locks interface {
	guard.NonBlockingLockerFactory[HitID]
}

var defaultLockerFactory = memory.NewLockerFactory[HitID]()

// Invalidator is a list of invalidation rule which is being used whenever an entity is being invalidated.
// It is ideal to invalidate a query by reconstucting the cache.HitID using the contents of the ENT OR ID.
type Invalidator[ENT, ID any] struct {
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
		for _, inv := range m.Invalidators {
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

	if err := m.InvalidateCachedQuery(ctx, m.HitIDFindByID(id)); err != nil {
		return err
	}

	hitsIter, err := m.Repository.Hits().FindAll(ctx)
	if err != nil {
		return err
	}

	HitsThatReferenceOurEntity := iterators.Filter[Hit[ID]](hitsIter, func(h Hit[ID]) bool {
		for _, gotID := range h.EntityIDs {
			if gotID == id {
				return true
			}
		}
		for _, inv := range m.Invalidators {
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
			ids = append(ids, h.ID)
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

func getAs[T any](src any) (T, error) {
	source, ok := src.(T)
	if !ok {
		return source, fmt.Errorf("%s: %w", reflectkit.TypeOf[T]().String(), ErrNotImplementedBySource)
	}
	return source, nil
}

func (m *Cache[ENT, ID]) RefreshByID(ctx context.Context, id ID) (rErr error) {
	return m.RefreshQueryOne(ctx, m.HitIDFindByID(id), func(ctx context.Context) (_ ENT, found bool, _ error) {
		return m.Source.FindByID(ctx, id)
	})
}

func (m *Cache[ENT, ID]) RefreshAll(ctx context.Context) (rErr error) {
	// MVP implementation
	source, err := getAs[crud.AllFinder[ENT]](m.Source)
	if err != nil {
		return err
	}
	return m.RefreshQueryMany(ctx, m.HitIDFindAll(), func(ctx context.Context) (iterators.Iterator[ENT], error) {
		return source.FindAll(ctx)
	})
}

func (m *Cache[ENT, ID]) RefreshQueryOne(ctx context.Context, hitID HitID, query QueryOneFunc[ENT]) (rErr error) {
	_, err := m.cacheQuery(ctx, hitID, m.mapQueryOneToQueryMany(query))
	return err
}

func (m *Cache[ENT, ID]) RefreshQueryMany(ctx context.Context, hitID HitID, query QueryManyFunc[ENT]) (rErr error) {
	_, err := m.cacheQuery(ctx, hitID, query)
	return err
}

func (m *Cache[ENT, ID]) InvalidateCachedQuery(ctx context.Context, hitID HitID) (rErr error) {
	ctx, err := m.Repository.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Repository, ctx)

	// it is important to first delete the hit record to avoid a loop effect with other invalidation calls.
	hit, found, err := m.invalidateCachedQueryWithoutCascadeEffect(ctx, hitID)
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

func (m *Cache[ENT, ID]) invalidateCachedQueryWithoutCascadeEffect(ctx context.Context, hitID HitID) (Hit[ID], bool, error) {
	hit, found, err := m.Repository.Hits().FindByID(ctx, hitID)
	if err != nil || !found {
		return hit, false, err
	}
	return hit, found, m.Repository.Hits().DeleteByID(ctx, hitID)
}

func (m *Cache[ENT, ID]) CachedQueryMany(ctx context.Context, hitID HitID, query QueryManyFunc[ENT]) (_ iterators.Iterator[ENT], rErr error) {
	// TODO: double check
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, ctx.Err()
		}
	}

	hit, found, err := m.Repository.Hits().FindByID(ctx, hitID)
	if err != nil {
		logger.Warn(ctx, fmt.Sprintf("error during retrieving hits for %s", hitID), logging.ErrField(err))
		return query(ctx)
	}
	if found {
		if len(hit.EntityIDs) == 0 {
			return iterators.Empty[ENT](), nil
		}

		m.doRefreshBehind(ctx, hitID, query)

		iter, err := m.Repository.Entities().FindByIDs(ctx, hit.EntityIDs...)
		const msg = "cache Repository.Entities().FindByIDs had an error"
		if err != nil {
			logger.Warn(ctx, msg, logging.ErrField(err))
			return query(ctx)
		}
		if err := iter.Err(); err != nil {
			if errors.Is(err, crud.ErrNotFound) {
				_ = m.Repository.Hits().DeleteByID(ctx, hit.ID)
			} else {
				logger.Warn(ctx, msg, logging.ErrField(err))
			}
			return query(ctx)
		}
		return iter, nil
	}

	ids, err := m.cacheQuery(ctx, hitID, query)
	if err != nil {
		logger.Warn(ctx, err.Error())
		return query(ctx)
	}

	result, err := m.Repository.Entities().FindByIDs(ctx, ids...)
	if err != nil {
		return query(ctx)
	}

	return result, nil
}

func (m *Cache[ENT, ID]) doRefreshBehind(ctx context.Context, hitID HitID, query QueryManyFunc[ENT]) {
	if !m.RefreshBehind {
		return
	}
	// refresh behind potentially finish after the request context is already cancelled,
	// we should not rely on the cancellation signal from the context.
	ctx = contextkit.WithoutCancel(ctx)
	// we want to avoid that the same query is continously executed parallel,
	// since we expect that the result would be the same.
	queryLock := m.locks().NonBlockingLockerFor(hitID)
	// tasker.WithNoOverlap ensures using the query lock that it actualy won't overlap
	task := tasker.WithNoOverlap(queryLock, func(ctx context.Context) error {
		_, err := m.cacheQuery(ctx, hitID, query)
		if err != nil {
			logger.Warn(ctx, err.Error())
		}
		return nil
	})
	// we simply execute the job in the background
	job := m.jobs.Background(ctx, task)
	// and then garbage collect its contents
	go job.Join()
}

// cacheQuery intentionally avoids using transactions to prioritise caching as many entities as possible.
// It works on a best-effort basis, focusing on maximising cache storage without enforcing strict transactional guarantees.
//
// If the Cache.Repository uses a “Repeatable Read” or higher isolation level,
// using transactions during a cache refresh would block reads from the cache, which we want to avoid.
func (m *Cache[ENT, ID]) cacheQuery(
	ctx context.Context,
	hitID HitID,
	query QueryManyFunc[ENT],
) (_ []ID, rErr error) {
	srcIter, err := query(ctx)
	if err != nil {
		return nil, err
	}
	defer srcIter.Close() // ignore err

	// intentionally an empty slice and not a nil slice to avoid it to be stored as null value in the Hits.
	var ids = make([]ID, 0)

	for srcIter.Next() {
		v := srcIter.Value()
		id, _ := m.IDA.Lookup(v)
		ids = append(ids, id)

		if err := m.Repository.Entities().Save(ctx, &v); err != nil {
			return nil, fmt.Errorf("cache Repository.Entities().Save had an error")
		}
	}

	if err := srcIter.Err(); err != nil {
		return nil, err
	}

	if err := m.Repository.Hits().Save(ctx, &Hit[ID]{
		ID:        hitID,
		EntityIDs: ids,
		Timestamp: clock.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("cache Repository.Hits().Save had an error")
	}

	return ids, nil
}

func (m *Cache[ENT, ID]) CachedQueryOne(
	ctx context.Context,
	hitID HitID,
	query QueryOneFunc[ENT],
) (_ent ENT, _found bool, _err error) {
	iter, err := m.CachedQueryMany(ctx, hitID, m.mapQueryOneToQueryMany(query))
	if err != nil {
		return _ent, false, err
	}
	ent, found, err := iterators.First[ENT](iter)
	if err != nil {
		return ent, false, err
	}
	if !found {
		return ent, false, nil
	}
	return ent, true, nil
}

func (m *Cache[ENT, ID]) mapQueryOneToQueryMany(q QueryOneFunc[ENT]) QueryManyFunc[ENT] {
	return func(ctx context.Context) (iterators.Iterator[ENT], error) {
		ent, found, err := q(ctx)
		if err != nil {
			return nil, err
		}
		if !found {
			return iterators.Empty[ENT](), nil
		}
		return iterators.SingleValue(ent), nil
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Cache[ENT, ID]) Idle() bool {
	return !m.jobs.Alive()
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

func (m *Cache[ENT, ID]) Save(ctx context.Context, ptr *ENT) error {
	source, ok := m.Source.(crud.Saver[ENT])
	if !ok {
		return fmt.Errorf("%s: %w", "Save", ErrNotImplementedBySource)
	}
	if err := source.Save(ctx, ptr); err != nil {
		return err
	}
	if err := m.Repository.Entities().Save(ctx, ptr); err != nil {
		logger.Warn(ctx, "cache Repository.Entities().Save had an error", logging.ErrField(err))
		if id, ok := m.IDA.Lookup(pointer.Deref(ptr)); ok {
			if err := m.InvalidateByID(ctx, id); err != nil {
				logger.Warn(ctx, "WARNING - "+
					"potentially invalid cache state. "+
					"After a failed Cache.Repository.Entities.Save, "+
					"the attempt to invalidate the entity by its id failed as well",
					logging.Field("type", reflectkit.TypeOf[ENT]().String()),
					logging.Field("id", id),
					logging.ErrField(err))
			}
		}
	}
	return nil
}

func (m *Cache[ENT, ID]) FindByID(ctx context.Context, id ID) (ENT, bool, error) {
	hitID := m.HitIDFindByID(id)
	query := func(ctx context.Context) (ent ENT, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	}
	// fast path
	ent, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		logger.Warn(ctx, "cache Repository.Entities().FindByID had an error", logging.ErrField(err))
		return m.Source.FindByID(ctx, id)
	}
	if found {
		m.doRefreshBehind(ctx, hitID, m.mapQueryOneToQueryMany(query))
		return ent, true, nil
	}
	// slow path
	return m.CachedQueryOne(ctx, hitID, query)
}

func (m *Cache[ENT, ID]) HitIDFindByID(id ID) HitID {
	return Query{
		Name:    "FindByID",
		ARGS:    map[string]any{"id": id},
		Version: 0,
	}.HitID()
}

func (m *Cache[ENT, ID]) HitIDFindAll() HitID {
	return Query{Name: "FindAll"}.HitID()
}

func (m *Cache[ENT, ID]) FindAll(ctx context.Context) (iterators.Iterator[ENT], error) {
	source, err := getAs[crud.AllFinder[ENT]](m.Source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "FindAll", ErrNotImplementedBySource)
	}
	return m.CachedQueryMany(ctx, m.HitIDFindAll(), func(ctx context.Context) (iterators.Iterator[ENT], error) {
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

func (m *Cache[ENT, ID]) locks() guard.NonBlockingLockerFactory[HitID] {
	if m.Locks != nil {
		return m.Locks
	}
	return defaultLockerFactory
}

func (m *Cache[ENT, ID]) Close() (rErr error) {
	return m.jobs.Stop()
}
