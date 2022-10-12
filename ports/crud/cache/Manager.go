package cache

import (
	"context"
	"fmt"
	"sync"

	"github.com/adamluzsi/frameless/pkg/errutils"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

func NewManager[Entity, ID any](repository Repository[Entity, ID], source Source[Entity, ID]) (*Manager[Entity, ID], error) {
	var r = &Manager[Entity, ID]{Repository: repository, Source: source}
	return r, r.Init(context.Background())
}

type Manager[Entity, ID any] struct {
	// Source is the location of the original data
	Source Source[Entity, ID]
	// Repository is the resource that keeps the cached data.
	Repository Repository[Entity, ID]

	init  sync.Once
	close func()
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Manager also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.Finder[Entity, ID]
	crud.Deleter[ID]
	pubsub.CreatorPublisher[Entity]
	pubsub.DeleterPublisher[ID]
}

type ExtendedSource[Entity, ID any] interface {
	crud.Updater[Entity]
	pubsub.UpdaterPublisher[Entity]
}

func (m *Manager[Entity, ID]) Init(ctx context.Context) error {
	var rErr error
	m.init.Do(func() { rErr = m.subscribe(ctx) })
	return rErr
}

func (m *Manager[Entity, ID]) trap(next func()) {
	if m.close == nil {
		m.close = func() {}
	}

	prev := m.close
	m.close = func() {
		prev()
		next()
	}
}

func (m *Manager[Entity, ID]) Close() error {
	if m.close == nil {
		return nil
	}
	m.close()
	return nil
}

func (m *Manager[Entity, ID]) entityTypeName() string {
	return reflects.SymbolicName(*new(Entity))
}

func (m *Manager[Entity, ID]) deleteCachedEntity(ctx context.Context, id ID) (rErr error) {
	s := m.Repository.CacheEntity(ctx)
	ctx, err := m.Repository.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rErr != nil {
			_ = m.Repository.RollbackTx(ctx)
			return
		}
		rErr = m.Repository.CommitTx(ctx)
	}()
	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return s.DeleteByID(ctx, id)
}

func (m *Manager[Entity, ID]) CacheQueryMany(
	ctx context.Context,
	name string,
	query func() iterators.Iterator[Entity],
) iterators.Iterator[Entity] {
	// TODO: double check
	if ctx != nil && ctx.Err() != nil {
		return iterators.Error[Entity](ctx.Err())
	}

	queryID := fmt.Sprintf(`0:%T/%s`, *new(Entity), name) // add version epoch
	hit, found, err := m.Repository.CacheHit(ctx).FindByID(ctx, queryID)
	if err != nil {
		return iterators.Error[Entity](err)
	}
	if found {
		// TODO: make sure that in case entity ids point to empty cache data
		//       we invalidate the hit and try again
		return m.Repository.CacheEntity(ctx).FindByIDs(ctx, hit.EntityIDs...)
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

	if err := m.Repository.CacheEntity(ctx).Upsert(ctx, vs...); err != nil {
		return iterators.Error[Entity](err)
	}

	if err := m.Repository.CacheHit(ctx).Create(ctx, &Hit[ID]{
		QueryID:   queryID,
		EntityIDs: ids,
	}); err != nil {
		return iterators.Error[Entity](err)
	}

	return iterators.Slice[Entity](res)
}

func (m *Manager[Entity, ID]) CacheQueryOne(
	ctx context.Context,
	queryID string,
	query func() (ent Entity, found bool, err error),
) (_ent Entity, _found bool, _err error) {
	iter := m.CacheQueryMany(ctx, queryID, func() iterators.Iterator[Entity] {
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

func (m *Manager[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	if err := m.Source.Create(ctx, ptr); err != nil {
		return err
	}
	return m.Repository.CacheEntity(ctx).Create(ctx, ptr)
}

func (m *Manager[Entity, ID]) FindByID(ctx context.Context, id ID) (Entity, bool, error) {
	// fast path
	ent, found, err := m.Repository.CacheEntity(ctx).FindByID(ctx, id)
	if err != nil {
		return ent, false, err
	}
	if found {
		return ent, true, nil
	}
	// slow path
	return m.CacheQueryOne(ctx, fmt.Sprintf(`FindByID#%v`, id), func() (ent Entity, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	})
}

func (m *Manager[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	return m.CacheQueryMany(ctx, `FindAll`, func() iterators.Iterator[Entity] {
		return m.Source.FindAll(ctx)
	})
}

func (m *Manager[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	switch src := m.Source.(type) {
	case ExtendedSource[Entity, ID]:
		if err := src.Update(ctx, ptr); err != nil {
			return err
		}
		return m.Repository.CacheEntity(ctx).Upsert(ctx, ptr)
	default:
		return errutils.Error(`not implemented`)
	}
}

func (m *Manager[Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := m.Source.DeleteByID(ctx, id); err != nil {
		return err
	}

	// TODO: unsafe without additional comproto layer
	dataRepository := m.Repository.CacheEntity(ctx)
	_, found, err := dataRepository.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := dataRepository.DeleteByID(ctx, id); err != nil {
		return err
	}
	if err := m.Repository.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Manager[Entity, ID]) DeleteAll(ctx context.Context) error {
	if err := m.Source.DeleteAll(ctx); err != nil {
		return err
	}
	return m.Repository.CacheEntity(ctx).DeleteAll(ctx)
}

func (m *Manager[Entity, ID]) SubscribeToCreatorEvents(ctx context.Context, creatorSubscriber pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error) {
	return m.Source.SubscribeToCreatorEvents(ctx, creatorSubscriber)
}

func (m *Manager[Entity, ID]) SubscribeToDeleterEvents(ctx context.Context, s pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	return m.Source.SubscribeToDeleterEvents(ctx, s)
}

func (m *Manager[Entity, ID]) SubscribeToUpdaterEvents(ctx context.Context, s pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error) {
	es, ok := m.Source.(ExtendedSource[Entity, ID])
	if !ok {
		return nil, fmt.Errorf("%T doesn't implement frameless.UpdaterPublisher", m.Source)
	}
	return es.SubscribeToUpdaterEvents(ctx, s)
}
