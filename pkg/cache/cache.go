// Package cache will supply caching solutions for your crud port compatible resources.
package cache

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase/clock"
)

func New[Entity, ID any](
	source Source[Entity, ID],
	cacheRepo Repository[Entity, ID],
) *Cache[Entity, ID] {
	return &Cache[Entity, ID]{
		Source:     source,
		Repository: cacheRepo,
	}
}

// Cache supplies Read/Write-Through caching to CRUD resources.
type Cache[Entity, ID any] struct {
	// Source is the location of the original data
	Source Source[Entity, ID]
	// Repository is the resource that keeps the cached data.
	Repository Repository[Entity, ID]
	// LogError is an OPTIONAL field, that you can supply if you want to inject your own logger.
	LogError func(ctx context.Context, msg string, err error)
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Cache also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source[Entity, ID any] interface {
	crud.ByIDFinder[Entity, ID]
}

type Repository[Entity, ID any] interface {
	Entities() EntityRepository[Entity, ID]
	Hits() HitRepository[ID]
	comproto.OnePhaseCommitProtocol
}

func (m *Cache[Entity, ID]) InvalidateByID(ctx context.Context, id ID) (rErr error) {
	ctx, err := m.Repository.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Repository, ctx)
	_, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	// brute force cache hit invalidation, could be easily fine-tuned later
	if err := m.Repository.Hits().DeleteAll(ctx); err != nil {
		return err
	}

	return m.Repository.Entities().DeleteByID(ctx, id)
}

func (m *Cache[Entity, ID]) DropCachedValues(ctx context.Context) error {
	return errorutil.Merge(
		m.Repository.Hits().DeleteAll(ctx),
		m.Repository.Entities().DeleteAll(ctx))
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
		m.logError(ctx, fmt.Sprintf("error during retrieving hits for %s", queryKey), err)
		return query()
	}
	if found {
		// TODO: make sure that in case entity ids point to empty cache data
		//       we invalidate the hit and try again
		iter := m.Repository.Entities().FindByIDs(ctx, hit.EntityIDs...)
		if err := iter.Err(); err != nil {
			m.logError(ctx, "cache Repository.Entities().FindByIDs had an error", err)
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
		vs = append(vs, &ent)
	}

	if err := m.Repository.Entities().Upsert(ctx, vs...); err != nil {
		m.logError(ctx, "cache Repository.Entities().Upsert had an error", err)
		return iterators.Slice[Entity](res)
	}

	if err := m.Repository.Hits().Create(ctx, &Hit[ID]{
		QueryID:   queryKey,
		EntityIDs: ids,
		Timestamp: clock.TimeNow().UTC(),
	}); err != nil {
		m.logError(ctx, "cache Repository.Hits().Create had an error", err)
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
		m.logError(ctx, "cache Repository.Entities().Create had an error", err)
	}
	return nil
}

func (m *Cache[Entity, ID]) FindByID(ctx context.Context, id ID) (Entity, bool, error) {
	// fast path
	ent, found, err := m.Repository.Entities().FindByID(ctx, id)
	if err != nil {
		m.logError(ctx, "cache Repository.Entities().FindByID had an error", err)
		return m.Source.FindByID(ctx, id)
	}
	if found {
		return ent, true, nil
	}
	// slow path
	key := QueryKey{
		ID:   "FindByID",
		ARGS: map[string]any{"ID": id},
	}
	return m.CachedQueryOne(ctx, key.Encode(), func() (ent Entity, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	})
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
		m.logError(ctx, "cache Repository.Entities().Update had an error", err)
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

func (m *Cache[Entity, ID]) logError(ctx context.Context, msg string, err error) {
	m.getLogError()(ctx, msg, err)
}

func (m *Cache[Entity, ID]) getLogError() func(context.Context, string, error) {
	if m.LogError != nil {
		return m.LogError
	}
	return m.defaultLogError
}

func (m *Cache[Entity, ID]) defaultLogError(ctx context.Context, msg string, err error) {
	logger.Error(ctx, msg, logger.Details{}.Err(err))
}
